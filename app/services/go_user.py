from __future__ import annotations

from fastapi import HTTPException


def _go_native_user_route():
    raise HTTPException(status_code=503, detail="User routes are served by the Go Master API")


def get_users_list(*args, **kwargs):
    _go_native_user_route()


def get_user_detail(*args, **kwargs):
    _go_native_user_route()
