import logging
import os
import sys
import warnings
from pathlib import Path

warnings.filterwarnings("ignore", message="pkg_resources is deprecated", category=UserWarning)
from fastapi import FastAPI, Request, status
from fastapi.encoders import jsonable_encoder
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from fastapi.routing import APIRoute

from config import ALLOWED_ORIGINS, DOCS, XRAY_SUBSCRIPTION_PATH

_PROTO_ROOT = Path(__file__).resolve().parent / "proto"
if _PROTO_ROOT.exists():
    sys.path.append(str(_PROTO_ROOT))
from app import runtime
from app.utils.system import register_scheduler_jobs

__version__ = "0.1.3"

IS_RUNNING_TESTS = "PYTEST_CURRENT_TEST" in os.environ or any(
    "pytest" in (arg or "").lower() for arg in sys.argv
)

SKIP_RUNTIME_INIT = os.getenv("REBECCA_SKIP_RUNTIME_INIT") == "1"
runtime.scheduler = None
runtime.app = None

logger = logging.getLogger("uvicorn.error")
runtime.logger = logger

# The master is node-only: Python must not bootstrap app.reb_node or a local
# Xray runtime. Node connectivity and runtime control are handled by Go/gRPC.
# Preserve test-provided runtime.xray mocks so legacy patch targets keep working.
if not hasattr(runtime, "xray"):
    runtime.xray = None

app = FastAPI(
    title="RebeccaAPI",
    description="Unified GUI Censorship Resistant Solution Powered by Xray",
    version=__version__,
    docs_url="/docs" if DOCS else None,
    redoc_url="/redoc" if DOCS else None,
)

if SKIP_RUNTIME_INIT:
    scheduler = None  # type: ignore[assignment]
else:
    from apscheduler.schedulers.background import BackgroundScheduler

    scheduler = BackgroundScheduler({"apscheduler.job_defaults.max_instances": 20}, timezone="UTC")
    runtime.scheduler = scheduler

runtime.app = app

if not SKIP_RUNTIME_INIT:
    from app.db.schema import ensure_runtime_schema

    ensure_runtime_schema()
allowed_origins = [origin.strip() for origin in ALLOWED_ORIGINS if origin.strip()]
if not allowed_origins:
    allowed_origins = ["*"]

allow_credentials = True
if "*" in allowed_origins:
    allowed_origins = ["*"]
    allow_credentials = False

app.add_middleware(
    CORSMiddleware,
    allow_origins=allowed_origins,
    allow_credentials=allow_credentials,
    allow_methods=["*"],
    allow_headers=["*"],
)
from app import routers  # noqa

if scheduler is not None:
    register_scheduler_jobs(scheduler)
from app.routers import api_router  # noqa

app.include_router(api_router)


def use_route_names_as_operation_ids(app: FastAPI) -> None:
    for route in app.routes:
        if isinstance(route, APIRoute):
            route.operation_id = route.name


if not SKIP_RUNTIME_INIT:
    use_route_names_as_operation_ids(app)


if not SKIP_RUNTIME_INIT:
    def on_startup():
        if IS_RUNNING_TESTS:
            return
        paths = [f"{r.path}/" for r in app.routes]
        paths.append("/api/")
        if f"/{XRAY_SUBSCRIPTION_PATH}/" in paths:
            raise ValueError(
                f"you can't use /{XRAY_SUBSCRIPTION_PATH}/ as subscription path it reserved for {app.title}"
            )

        # Start scheduler first (so server can start quickly)
        scheduler.start()

    def on_shutdown():
        if IS_RUNNING_TESTS:
            return
        if scheduler:
            scheduler.shutdown()

    app.add_event_handler("startup", on_startup)
    app.add_event_handler("shutdown", on_shutdown)

    @app.exception_handler(RequestValidationError)
    def validation_exception_handler(request: Request, exc: RequestValidationError):
        details = {}
        for error in exc.errors():
            details[error["loc"][-1]] = error.get("msg")
        return JSONResponse(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            content=jsonable_encoder({"detail": details}),
        )
