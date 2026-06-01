from __future__ import annotations

import threading

from app.runtime import logger
from app.services.go_usage import GoUsageError, GoUsageUnavailable, call_bridge


_record_user_usages_lock = threading.Lock()
_record_node_usages_lock = threading.Lock()


def _collect_usage(*, users: bool, outbound: bool) -> dict | None:
    try:
        result = call_bridge(
            "usage.collect",
            {
                "users": bool(users),
                "outbound": bool(outbound),
                "reset": True,
            },
        )
        if isinstance(result, dict) and result.get("errors"):
            logger.warning("Go usage collection completed with errors: %s", result.get("errors"))
        return result if isinstance(result, dict) else None
    except (GoUsageError, GoUsageUnavailable) as exc:
        logger.warning("Go usage collection failed: %s", exc)
        return None


def record_user_usages():
    if not _record_user_usages_lock.acquire(blocking=False):
        logger.warning("record_user_usages is already running; skipping overlapping run")
        return None
    try:
        return _collect_usage(users=True, outbound=False)
    finally:
        _record_user_usages_lock.release()


def record_node_usages():
    if not _record_node_usages_lock.acquire(blocking=False):
        logger.warning("record_node_usages is already running; skipping overlapping run")
        return None
    try:
        return _collect_usage(users=False, outbound=True)
    finally:
        _record_node_usages_lock.release()
