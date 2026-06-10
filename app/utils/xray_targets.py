from __future__ import annotations

from copy import deepcopy

from fastapi import HTTPException
from sqlalchemy.orm import Session

from app.db import crud
from app.db.models import Node as DBNode
from app.models.node import XrayConfigMode
from app.utils.xray_defaults import apply_log_paths


MASTER_TARGET_ID = "master"
NODE_TARGET_PREFIX = "node:"


def node_target_id(node_id: int) -> str:
    return f"{NODE_TARGET_PREFIX}{int(node_id)}"


def parse_target_id(target_id: str | None) -> tuple[str, int | None]:
    target = (target_id or MASTER_TARGET_ID).strip()
    if target == MASTER_TARGET_ID:
        return MASTER_TARGET_ID, None
    if target.startswith(NODE_TARGET_PREFIX):
        raw_id = target[len(NODE_TARGET_PREFIX) :]
        try:
            return NODE_TARGET_PREFIX.rstrip(":"), int(raw_id)
        except (TypeError, ValueError):
            pass
    raise HTTPException(status_code=400, detail="Invalid Xray config target")


def normalize_config_payload(payload: dict | None) -> dict:
    return apply_log_paths(deepcopy(payload or {}))


def node_config_mode(dbnode: DBNode) -> XrayConfigMode:
    value = getattr(dbnode, "xray_config_mode", XrayConfigMode.default)
    try:
        return XrayConfigMode(value)
    except ValueError:
        return XrayConfigMode.default


def node_uses_custom_config(dbnode: DBNode) -> bool:
    return node_config_mode(dbnode) == XrayConfigMode.custom


def get_node_effective_raw_config(
    dbnode: DBNode,
    master_config: dict | None = None,
) -> dict:
    if node_uses_custom_config(dbnode) and isinstance(getattr(dbnode, "xray_config", None), dict):
        return normalize_config_payload(dbnode.xray_config)
    return normalize_config_payload(master_config or {})


def get_target_raw_config(db: Session, target_id: str | None = None) -> dict:
    kind, node_id = parse_target_id(target_id)
    master_config = crud.get_xray_config(db)
    if kind == MASTER_TARGET_ID:
        return normalize_config_payload(master_config)

    dbnode = crud.get_node_by_id(db, node_id)
    if not dbnode:
        raise HTTPException(status_code=404, detail="Node not found")
    return get_node_effective_raw_config(dbnode, master_config)


def iter_stored_raw_configs(db: Session) -> list[tuple[str, dict]]:
    configs: list[tuple[str, dict]] = [(MASTER_TARGET_ID, crud.get_xray_config(db))]
    for node in crud.get_nodes(db):
        if node_uses_custom_config(node) and isinstance(getattr(node, "xray_config", None), dict):
            configs.append((node_target_id(node.id), node.xray_config))
    return [(target_id, normalize_config_payload(config)) for target_id, config in configs]
