import pytest
from fastapi import HTTPException

from app.routers import runtime


def test_test_outbound_rejects_blackhole():
    outbound = {"tag": "blocked", "protocol": "blackhole"}

    payload = runtime.test_outbound(payload={"outbound": outbound}, admin=object())

    assert payload["success"] is True
    assert payload["obj"]["success"] is False
    assert "cannot be tested" in payload["obj"]["error"].lower()


def test_test_outbound_invalid_json_returns_400():
    with pytest.raises(HTTPException) as exc:
        runtime.test_outbound(payload={"outbound": "{invalid"}, admin=object())

    assert exc.value.status_code == 400
    assert "Invalid outbound JSON" in exc.value.detail
