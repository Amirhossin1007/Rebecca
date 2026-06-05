from __future__ import annotations

from fastapi import HTTPException


def generate_v2ray_subscription(*args, **kwargs) -> str:
    raise HTTPException(status_code=503, detail="Subscription routes are served by the Go Master API")
