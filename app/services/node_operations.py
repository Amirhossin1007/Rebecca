from __future__ import annotations

import hashlib
import json
import logging
from datetime import UTC, datetime
from typing import Any

from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.db import GetDB, crud
from app.db.models import Node, NodeOperation, User
from app.models.node import NodeStatus
from app.services.go_usage import GoUsageError, GoUsageUnavailable, call_bridge

logger = logging.getLogger(__name__)

PENDING = "pending"

SYNC_CONFIG = "sync_config"
ADD_USER = "add_user"
UPDATE_USER = "update_user"
REMOVE_USER = "remove_user"
DISABLE_USER = "disable_user"
ENABLE_USER = "enable_user"
RESTART_NODE = "restart_node"

CONFIG_OPERATIONS = {
    SYNC_CONFIG,
    ADD_USER,
    UPDATE_USER,
    REMOVE_USER,
    DISABLE_USER,
    ENABLE_USER,
}


def _canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def _idempotency_key(operation_type: str, *, node_id: int | None, user_id: int | None, payload: dict[str, Any]) -> str:
    raw = _canonical_json(
        {
            "operation_type": operation_type,
            "node_id": node_id,
            "user_id": user_id,
            "payload": payload,
        }
    )
    return hashlib.sha256(raw.encode()).hexdigest()


def _enabled_nodes(db: Session) -> list[Node]:
    return crud.get_nodes(db, enabled=True)


def enqueue_config_operations(
    db: Session,
    *,
    operation_type: str,
    user_id: int | None = None,
    node_id: int | None = None,
) -> int:
    if operation_type not in CONFIG_OPERATIONS:
        raise ValueError(f"Unsupported config operation: {operation_type}")

    nodes = [crud.get_node_by_id(db, node_id)] if node_id is not None else _enabled_nodes(db)
    nodes = [node for node in nodes if node is not None and node.status not in (NodeStatus.disabled, NodeStatus.limited)]
    if not nodes:
        return 0

    created = 0
    queued_at = datetime.now(UTC).isoformat()
    for node in nodes:
        payload: dict[str, Any] = {"queued_at": queued_at}
        idempotency_key = _idempotency_key(
            operation_type,
            node_id=node.id,
            user_id=user_id,
            payload=payload,
        )
        if db.query(NodeOperation.id).filter(NodeOperation.idempotency_key == idempotency_key).first():
            continue
        op = NodeOperation(
            operation_type=operation_type,
            node_id=node.id,
            user_id=user_id,
            payload=payload,
            status=PENDING,
            idempotency_key=idempotency_key,
        )
        db.add(op)
        created += 1

    try:
        db.commit()
    except IntegrityError:
        db.rollback()
        logger.info("Duplicate node operation skipped during enqueue")
    return created


def enqueue_restart_operation(db: Session, node_id: int, *, config_json: str | None = None) -> int:
    node = crud.get_node_by_id(db, node_id)
    if not node:
        return 0
    payload = {"queued_at": datetime.now(UTC).isoformat()}
    if config_json:
        payload["config_json"] = config_json
    op = NodeOperation(
        operation_type=RESTART_NODE,
        node_id=node.id,
        payload=payload,
        status=PENDING,
        idempotency_key=_idempotency_key(RESTART_NODE, node_id=node.id, user_id=None, payload=payload),
    )
    db.add(op)
    try:
        db.commit()
    except IntegrityError:
        db.rollback()
        return 0
    return 1


def queue_user_operation(user_id: int, operation_type: str) -> int:
    with GetDB() as db:
        actual_user_id = int(user_id)
        if not db.query(User.id).filter(User.id == actual_user_id).first():
            actual_user_id = None
        created = enqueue_config_operations(db, operation_type=operation_type, user_id=actual_user_id)
    process_pending_operations()
    return created


def queue_sync_config(node_id: int | None = None) -> int:
    with GetDB() as db:
        created = enqueue_config_operations(db, operation_type=SYNC_CONFIG, node_id=node_id)
    process_pending_operations()
    return created


def queue_restart_node(node_id: int, *, config_json: str | None = None) -> int:
    with GetDB() as db:
        created = enqueue_restart_operation(db, int(node_id), config_json=config_json)
    process_pending_operations()
    return created


def process_pending_operations(*, limit: int = 50, node_id: int | None = None) -> dict[str, Any] | None:
    payload: dict[str, Any] = {"limit": int(limit)}
    if node_id is not None:
        payload["node_id"] = int(node_id)
    try:
        result = call_bridge("node.operations.process", payload)
        if isinstance(result, dict):
            return result
    except (GoUsageError, GoUsageUnavailable) as exc:
        logger.warning("Failed to process node operations: %s", exc)
    return None
