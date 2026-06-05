import os
from pathlib import Path
import tempfile
import uuid

TEST_DB_PATH = Path(tempfile.gettempdir()) / f"rebecca_test_{uuid.uuid4().hex}.sqlite"
TEST_DATABASE_URL = os.getenv("REBECCA_TEST_DATABASE_URL") or f"sqlite:///{TEST_DB_PATH}"
EXTERNAL_TEST_DATABASE = bool(os.getenv("REBECCA_TEST_DATABASE_URL"))
TEST_ENV_PATH = Path(tempfile.gettempdir()) / f"rebecca_test_{uuid.uuid4().hex}.env"
TEST_ENV_PATH.write_text(
    "\n".join(
        [
            f"SQLALCHEMY_DATABASE_URL={TEST_DATABASE_URL}",
            "REBECCA_SKIP_RUNTIME_INIT=1",
        ]
    )
    + "\n",
    encoding="utf-8",
)

os.environ.setdefault("REBECCA_SKIP_RUNTIME_INIT", "1")
os.environ["REBECCA_ENV_FILE"] = str(TEST_ENV_PATH)
# Keep the application engine and the pytest fixture engine on the same database.
os.environ["SQLALCHEMY_DATABASE_URL"] = TEST_DATABASE_URL

import sys
import warnings

# Silence noisy third-party deprecation warnings early, before imports trigger them
warnings.filterwarnings(
    "ignore",
    category=PendingDeprecationWarning,
    module="starlette.formparsers",
    message=".*import python_multipart.*",
)

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from unittest.mock import patch, MagicMock


sys.modules["app.proto"] = MagicMock()
sys.modules["app.proto.rebecca"] = MagicMock()
sys.modules["app.proto.rebecca.app"] = MagicMock()
sys.modules["app.proto.rebecca.app.router"] = MagicMock()
sys.modules["app.proto.rebecca.app.router.config_pb2"] = MagicMock()

# Silence noisy third-party deprecation warnings in test output
warnings.filterwarnings(
    "ignore",
    category=DeprecationWarning,
    module="lark.utils",
    message=".*sre_parse.*",
)
warnings.filterwarnings(
    "ignore",
    category=DeprecationWarning,
    module="lark.utils",
    message=".*sre_constants.*",
)

# Patch xray before any imports
mock_xray = MagicMock()
mock_xray.config.inbounds_by_protocol = {"vmess": [{"tag": "VMess TCP"}], "vless": [{"tag": "VLESS TCP"}]}
mock_xray.config.inbounds_by_tag = {
    "VMess TCP": {"tag": "VMess TCP", "protocol": "vmess"},
    "VLESS TCP": {"tag": "VLESS TCP", "protocol": "vless"},
}
mock_xray.config.include_db_users = MagicMock(return_value=MagicMock())
mock_xray.operations.remove_user = MagicMock()
mock_xray.operations.restart_node = MagicMock()
mock_xray.nodes = {}

# Patch TelegramSettingsService to avoid external DB connections in tests
patch("app.utils.report._event_enabled", return_value=False).start()

import app.runtime

app.runtime.xray = mock_xray

# Mock get_public_ip and xray before importing app
test_app = None
with (
    patch("app.utils.system.get_public_ip", return_value="127.0.0.1"),
):
    from app import app as _app

    if _app is None:
        # Create app manually
        from fastapi import FastAPI
        from app.routers import api_router

        test_app = FastAPI(title="RebeccaAPI", docs_url=None, redoc_url=None)
        test_app.include_router(api_router)
    else:
        test_app = _app
    import app as app_pkg

    app_pkg.app = test_app
    from app.db.base import Base
    from app.db import get_db
    import app.db.models  # noqa: F401  # Import models to register tables

    # Import models to register tables


engine_kwargs = {}
if TEST_DATABASE_URL.startswith("sqlite"):
    engine_kwargs["connect_args"] = {"check_same_thread": False}
elif EXTERNAL_TEST_DATABASE:
    # Use per-statement visibility for cross-session assertions on MySQL/MariaDB.
    engine_kwargs["isolation_level"] = "READ COMMITTED"
    engine_kwargs["pool_pre_ping"] = True

engine = create_engine(TEST_DATABASE_URL, **engine_kwargs)
TestingSessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)


@pytest.fixture(scope="session", autouse=True)
def db_engine():
    if not EXTERNAL_TEST_DATABASE:
        Base.metadata.create_all(bind=engine)
    yield engine
    if not EXTERNAL_TEST_DATABASE:
        Base.metadata.drop_all(bind=engine)


@pytest.fixture(scope="session", autouse=True)
def setup_test_admin(db_engine):
    # Create test admin
    db = TestingSessionLocal()
    from app.db.crud import get_admin

    existing = get_admin(db, "testadmin")
    if not existing:
        from app.db.crud import create_admin
        from app.models.admin import AdminCreate, AdminRole

        admin_data = AdminCreate(username="testadmin", password="testpass", role=AdminRole.full_access)
        create_admin(db, admin_data)
        db.commit()
    db.close()


@pytest.fixture(scope="session")
def client(setup_test_admin):
    # Override the database dependency to use test database
    def override_get_db():
        try:
            db = TestingSessionLocal()
            yield db
        finally:
            db.close()

    test_app.dependency_overrides[get_db] = override_get_db
    with TestClient(test_app) as c:
        yield c


@pytest.fixture(scope="session")
def auth_client(client):
    client.headers.update(admin_auth_headers("testadmin"))
    return client


def admin_auth_headers(username: str, role: str = "standard") -> dict[str, str]:
    import jwt
    from datetime import datetime, timezone

    data = {"sub": username, "role": role, "iat": datetime.now(timezone.utc)}
    token = jwt.encode(data, _test_admin_secret_key(), algorithm="HS256")
    return {"Authorization": f"Bearer {token}"}


def _test_admin_secret_key() -> str:
    from app.db.crud import get_admin_secret_key

    db = TestingSessionLocal()
    try:
        return get_admin_secret_key(db)
    finally:
        db.close()


def _test_admin_payload(token: str):
    import jwt
    from datetime import datetime, timezone

    try:
        payload = jwt.decode(token, _test_admin_secret_key(), algorithms=["HS256"])
    except jwt.exceptions.PyJWTError:
        return None
    username = payload.get("sub")
    role_value = payload.get("role") or payload.get("access")
    if not username or not role_value:
        return None
    if role_value == "admin":
        role_value = "standard"
    elif role_value not in ("standard", "sudo", "full_access"):
        return None
    try:
        created_at = datetime.fromtimestamp(payload["iat"], timezone.utc)
    except KeyError:
        created_at = None
    return {"username": username, "role": role_value, "created_at": created_at}


@pytest.fixture(autouse=True)
def mock_go_admin_validation(monkeypatch):
    from fastapi import HTTPException

    from app.db import crud
    from app.models.admin import Admin, AdminStatus
    from app.services import go_master_api

    original_request_json = go_master_api.request_json

    def fake_request_json(method, path, *, authorization=None, json_body=None, **kwargs):
        if path != "/internal/admin/validate":
            return original_request_json(method, path, authorization=authorization, json_body=json_body, **kwargs)

        token = ""
        if authorization and authorization.lower().startswith("bearer "):
            token = authorization[7:].strip()
        if not token and isinstance(json_body, dict):
            token = str(json_body.get("token") or "").strip()
        if not token:
            raise HTTPException(status_code=401, detail="missing bearer token")

        db = TestingSessionLocal()
        try:
            payload = _test_admin_payload(token)
            if payload:
                dbadmin = crud.get_admin(db, payload["username"])
                if not dbadmin:
                    raise HTTPException(status_code=401, detail="admin not found")
                admin_payload = Admin.model_validate(dbadmin).model_dump(mode="json")
                return {"valid": True, "source": "jwt", "admin": admin_payload}

            api_key = crud.get_admin_api_key_by_token(db, token)
            if not api_key:
                raise HTTPException(status_code=401, detail="invalid admin token")
            dbadmin = crud.get_admin_by_id(db, api_key.admin_id)
            if not dbadmin or dbadmin.status != AdminStatus.active:
                raise HTTPException(status_code=401, detail="admin is not active")
            admin_payload = Admin.model_validate(dbadmin).model_dump(mode="json")
            return {"valid": True, "source": "api_key", "admin": admin_payload}
        finally:
            db.close()

    monkeypatch.setattr(go_master_api, "request_json", fake_request_json)


@pytest.fixture
def xray_mock():
    """Provides access to the mock xray for assertions in tests"""
    # Reset call counts before each test
    mock_xray.operations.restart_node.reset_mock()
    mock_xray.operations.remove_user.reset_mock()
    mock_xray.config.include_db_users.reset_mock()
    return mock_xray
