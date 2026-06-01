from typing import Any, Dict

from fastapi import HTTPException

from app.db import GetDB, crud
from app.models.node import XrayConfigMode
from app.services import node_operations
from app.utils.xray_targets import (
    MASTER_TARGET_ID,
    node_target_id,
    parse_target_id,
    save_target_raw_config,
)
from app.xray.config import XRayConfig

DEFAULT_XRAY_API_PORT = 8080


def _validate_config(payload: dict) -> XRayConfig:
    try:
        return XRayConfig(payload, api_port=DEFAULT_XRAY_API_PORT)
    except ValueError as err:
        raise HTTPException(status_code=400, detail=str(err)) from err


def restart_default_runtimes_and_invalidate_cache(startup_config=None):
    del startup_config
    restart_runtime_targets()


def restart_runtime_targets(target_ids: set[str] | list[str] | tuple[str, ...] | None = None) -> None:
    """
    Queue node config syncs for default/custom runtime targets.
    The master has no local Xray runtime.
    """
    normalized_targets = set(target_ids or [])
    restart_all = not normalized_targets
    restart_master = restart_all or MASTER_TARGET_ID in normalized_targets

    with GetDB() as db:
        dbnodes = crud.get_nodes(db=db, enabled=True)
        node_ids: list[int] = []
        for dbnode in dbnodes:
            target_id = node_target_id(dbnode.id)
            mode = getattr(dbnode, "xray_config_mode", XrayConfigMode.default)
            uses_default = mode == XrayConfigMode.default
            if restart_all or target_id in normalized_targets or (restart_master and uses_default):
                node_ids.append(int(dbnode.id))

    for node_id in node_ids:
        node_operations.queue_sync_config(node_id=node_id)


def apply_config(payload: Dict[str, Any]) -> XRayConfig:
    """
    Persist a new default Xray configuration. Runtime application is handled by
    Go/gRPC node operations.
    """
    config = _validate_config(payload)
    with GetDB() as db:
        crud.save_xray_config(db, payload)
    return config


def apply_runtime_config_and_restart(payload: Dict[str, Any]) -> None:
    apply_config(payload)
    restart_runtime_targets()


def apply_target_runtime_config_and_restart(target_id: str, payload: Dict[str, Any]) -> None:
    _validate_config(payload)
    with GetDB() as db:
        save_target_raw_config(db, target_id, payload)
    restart_runtime_targets({target_id})


def soft_reload_panel():
    """
    Soft reload panel configuration without touching a local runtime.
    Nodes converge through the Go operation queue.
    """
    import logging

    logger = logging.getLogger("uvicorn.error")
    with GetDB() as db:
        raw_config = crud.get_xray_config(db)

    _validate_config(raw_config)
    try:
        node_operations.queue_sync_config()
    except Exception as exc:
        logger.error("Failed to queue node sync during soft reload: %s", exc)
