"""
User service layer.

User runtime paths are served by the Go Master API. This module only keeps
transitional serialization helpers for imports/tests that still reference them.
"""

from typing import List, Optional

from app.db import Session
from app.db.models import User
from app.models.user import UserCreate, UserListItem, UserModify, UserResponse, UserStatus, UsersResponse
from app.utils.subscription_links import build_subscription_links
from config import USERS_LIST_SUBSCRIPTION_URLS_ENABLED


def _go_native_user_route():
    raise RuntimeError("User routes are served by the Go Master API")


def _compute_primary_subscription_link(
    username: str,
    credential_key: Optional[str],
    *,
    subadress: Optional[str] = None,
    list_link_context: Optional[dict] = None,
) -> tuple[str, dict]:
    preferred = "key"
    if list_link_context:
        preferred = list_link_context.get("default_subscription_type") or preferred

    subadress = (subadress or "").strip()
    if subadress:
        url = f"/sub/{subadress}"
        return url, {"subadress": url}

    if credential_key and preferred == "username-key":
        url = f"/sub/{username}/{credential_key}"
        return url, {"username-key": url}

    if credential_key:
        url = f"/sub/{credential_key}"
        return url, {"key": url}

    return "", {}


def _compute_subscription_links(
    username: str,
    credential_key: Optional[str],
    *,
    admin=None,
    admin_id: Optional[int] = None,
    request_origin: Optional[str] = None,
    subadress: Optional[str] = None,
    list_link_context: Optional[dict] = None,
) -> tuple[str, dict]:
    if not USERS_LIST_SUBSCRIPTION_URLS_ENABLED:
        return _compute_primary_subscription_link(
            username,
            credential_key,
            subadress=subadress,
            list_link_context=list_link_context,
        )

    try:
        payload = type(
            "UserListSubscriptionPayload",
            (),
            {
                "username": username,
                "credential_key": credential_key,
                "admin": admin,
                "admin_id": admin_id,
                "subadress": subadress or "",
            },
        )()
        links = build_subscription_links(payload, request_origin=request_origin)
        primary = links.get("primary", "")
        return primary or "", links
    except Exception:
        return _compute_primary_subscription_link(
            username,
            credential_key,
            subadress=subadress,
            list_link_context=list_link_context,
        )


def _map_raw_to_list_item(
    raw: dict,
    include_links: bool = False,
    admin_lookup: Optional[dict] = None,
    request_origin: Optional[str] = None,
    list_link_context: Optional[dict] = None,
) -> UserListItem:
    del include_links
    admin_obj = admin_lookup.get(raw.get("admin_id")) if admin_lookup else None
    subscription_url, subscription_urls = _compute_subscription_links(
        raw.get("username", ""),
        raw.get("credential_key"),
        admin=admin_obj,
        admin_id=raw.get("admin_id"),
        request_origin=request_origin,
        subadress=raw.get("subadress"),
        list_link_context=list_link_context,
    )

    return UserListItem(
        username=raw.get("username"),
        status=raw.get("status"),
        used_traffic=raw.get("used_traffic") or 0,
        lifetime_used_traffic=raw.get("lifetime_used_traffic") or 0,
        created_at=raw.get("created_at"),
        expire=raw.get("expire"),
        data_limit=raw.get("data_limit"),
        data_limit_reset_strategy=raw.get("data_limit_reset_strategy"),
        online_at=raw.get("online_at"),
        service_id=raw.get("service_id"),
        service_name=raw.get("service_name"),
        admin_id=raw.get("admin_id"),
        admin_username=raw.get("admin_username"),
        links=[],
        subscription_url=subscription_url,
        subscription_urls=subscription_urls,
    )


def get_users_list(
    db: Session,
    *,
    offset: Optional[int],
    limit: Optional[int],
    username: Optional[List[str]],
    search: Optional[str],
    status: Optional[UserStatus],
    sort,
    advanced_filters,
    service_id: Optional[int],
    dbadmin,
    owners: Optional[List[str]],
    users_limit: Optional[int],
    active_total: Optional[int],
    include_links: bool = False,
    request_origin: Optional[str] = None,
) -> UsersResponse:
    _go_native_user_route()


def get_users_list_db_only(
    db: Session,
    *,
    offset: Optional[int],
    limit: Optional[int],
    username: Optional[List[str]],
    search: Optional[str],
    status: Optional[UserStatus],
    sort,
    advanced_filters,
    service_id: Optional[int],
    dbadmin,
    owners: Optional[List[str]],
    users_limit: Optional[int],
    active_total: Optional[int],
    include_links: bool = False,
    request_origin: Optional[str] = None,
) -> UsersResponse:
    """Compatibility wrapper for callers that still use the old function name."""
    _go_native_user_route()


def get_user_detail(username: str, db: Session) -> Optional[UserResponse]:
    _go_native_user_route()


def create_user(db: Session, payload: UserCreate, admin=None, service=None) -> UserResponse:
    _go_native_user_route()


def update_user(db: Session, dbuser: User, payload: UserModify) -> UserResponse:
    _go_native_user_route()


def delete_user(db: Session, dbuser: User):
    _go_native_user_route()
