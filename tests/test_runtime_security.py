import socket

import pytest
from fastapi import HTTPException

from app.routers.runtime import _safe_geo_filename, _validate_download_url


def test_safe_geo_filename_only_allows_expected_assets():
    assert _safe_geo_filename("geoip.dat") == "geoip.dat"
    assert _safe_geo_filename("nested/geosite.dat") == "geosite.dat"

    with pytest.raises(HTTPException) as exc:
        _safe_geo_filename("../../authorized_keys")

    assert exc.value.status_code == 422


def test_validate_download_url_rejects_private_resolved_addresses(monkeypatch):
    monkeypatch.setattr(
        socket,
        "getaddrinfo",
        lambda *args, **kwargs: [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("127.0.0.1", 443))],
    )

    with pytest.raises(HTTPException) as exc:
        _validate_download_url("https://example.com/file.dat")

    assert exc.value.status_code == 422
