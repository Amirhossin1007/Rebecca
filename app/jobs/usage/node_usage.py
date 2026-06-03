"""
Legacy compatibility helpers for node usage tests and transition-only imports.

Runtime node usage collection is scheduled through app.jobs.usage.go_collector.
This module keeps the old buffered write path available for tests without
starting the Python node runtime.
"""

from concurrent.futures import ThreadPoolExecutor, wait
import contextlib
from functools import partial
import threading
from typing import Union

from sqlalchemy import and_, bindparam, insert, select, text, update
from sqlalchemy.exc import OperationalError, TimeoutError as SQLTimeoutError

from app.db import GetDB
from app.db.models import Node, NodeUsage, System
from app.jobs.usage.collectors import get_outbounds_stats, resolve_stats_api
from app.jobs.usage.delivery_buffer import usage_delivery_buffer
from app.jobs.usage.outbound_traffic import _persist_outbound_traffic
from app.jobs.usage.utils import hour_bucket, is_retryable_db_error, retry_delay, safe_execute, utcnow_naive
from app.models.node import NodeResponse, NodeStatus
from app.runtime import logger
from app.utils import report
from config import (
    DISABLE_RECORDING_NODE_USAGE,
    JOB_RECORD_NODE_USAGE_COLLECT_TIMEOUT,
    JOB_RECORD_NODE_USAGE_WORKERS,
    JOB_USAGE_DB_LOCK_WAIT_TIMEOUT,
    JOB_USAGE_DB_MAX_RETRIES,
)


_record_node_usages_lock = threading.Lock()


class _RuntimeXrayProxy:
    """Live compatibility target for legacy outbound usage tests."""

    def __getattr__(self, name):
        from app import runtime as runtime_state

        target = runtime_state.xray
        if target is None:
            raise AttributeError(name)
        return getattr(target, name)


xray = _RuntimeXrayProxy()


def _set_usage_lock_wait_timeout(db) -> None:
    timeout = int(JOB_USAGE_DB_LOCK_WAIT_TIMEOUT or 0)
    if timeout <= 0:
        return
    try:
        if getattr(db.bind, "name", "") == "mysql":
            db.execute(text("SET SESSION innodb_lock_wait_timeout = :timeout"), {"timeout": timeout})
    except Exception as exc:  # pragma: no cover - advisory tuning only
        logger.debug("Failed to set node usage DB lock wait timeout: %s", exc)


def _nodes():
    return getattr(xray, "nodes", {}) or {}


def _update_node_limits(db, dbnode: Node, total_up: int, total_down: int, *, commit: bool = True):
    limited_triggered = False
    limit_cleared = False
    status_change_payload = None

    dbnode.uplink = (dbnode.uplink or 0) + total_up
    dbnode.downlink = (dbnode.downlink or 0) + total_down

    current_usage = (dbnode.uplink or 0) + (dbnode.downlink or 0)
    limit = dbnode.data_limit

    if limit is not None and current_usage >= limit:
        if dbnode.status != NodeStatus.limited:
            previous_status = dbnode.status
            dbnode.status = NodeStatus.limited
            dbnode.message = "Data limit reached"
            dbnode.xray_version = None
            dbnode.last_status_change = utcnow_naive()
            limited_triggered = True
            status_change_payload = (NodeResponse.model_validate(dbnode), previous_status)
    else:
        if dbnode.status == NodeStatus.limited:
            previous_status = dbnode.status
            dbnode.status = NodeStatus.connecting
            dbnode.message = None
            dbnode.xray_version = None
            dbnode.last_status_change = utcnow_naive()
            limit_cleared = True
            status_change_payload = (NodeResponse.model_validate(dbnode), previous_status)

    if commit:
        db.commit()
    return limited_triggered, limit_cleared, status_change_payload


def _is_missing_node_usage_endpoint(exc: Exception) -> bool:
    return getattr(exc, "status_code", None) in (404, 405)


def _collect_node_outbound_stats(node):
    api = resolve_stats_api(node)
    if api is not None:
        return {"stats": get_outbounds_stats(api), "node_batch_id": ""}

    if not hasattr(node, "collect_outbound_stats"):
        raise RuntimeError("Node does not support buffered outbound usage collection")

    try:
        payload = node.collect_outbound_stats()
    except Exception as exc:
        if _is_missing_node_usage_endpoint(exc):
            raise RuntimeError("Node does not support buffered outbound usage collection") from exc
        raise

    return {
        "stats": payload.get("stats") or [],
        "node_batch_id": payload.get("batch_id") or "",
    }


def _ack_node_outbound_batches(node_batches: dict[int, str]) -> None:
    for node_id, batch_id in node_batches.items():
        if not batch_id:
            continue
        node = _nodes().get(node_id)
        if not node:
            continue
        try:
            node.ack_outbound_stats(batch_id)
        except Exception as exc:  # pragma: no cover - best effort
            logger.warning("Failed to ack outbound usage batch %s for node %s: %s", batch_id, node_id, exc)


def record_node_stats(params: dict, node_id: Union[int, None]):
    if not params:
        return

    total_up = sum(item.get("up", 0) for item in params)
    total_down = sum(item.get("down", 0) for item in params)

    limited_triggered = False
    limit_cleared = False
    status_change_payload = None
    created_at = hour_bucket()

    with GetDB() as db:
        dbnode = None
        if node_id is not None:
            dbnode = db.query(Node).filter(Node.id == node_id).with_for_update().first()
            if dbnode is None:
                logger.warning("Skipping node usage for missing node id %s", node_id)
                return

        select_stmt = select(NodeUsage.node_id).where(
            and_(NodeUsage.node_id == node_id, NodeUsage.created_at == created_at)
        )
        notfound = db.execute(select_stmt).first() is None
        if notfound:
            stmt = insert(NodeUsage).values(created_at=created_at, node_id=node_id, uplink=0, downlink=0)
            safe_execute(db, stmt)

        stmt = (
            update(NodeUsage)
            .values(uplink=NodeUsage.uplink + bindparam("up"), downlink=NodeUsage.downlink + bindparam("down"))
            .where(and_(NodeUsage.node_id == node_id, NodeUsage.created_at == created_at))
        )
        safe_execute(db, stmt, params)

        if dbnode is not None and (total_up or total_down):
            limited_triggered, limit_cleared, status_change_payload = _update_node_limits(
                db, dbnode, total_up, total_down
            )

    if status_change_payload:
        node_resp, prev_status = status_change_payload
        report.node_status_change(node_resp, previous_status=prev_status)

    return None


def _persist_node_stats_in_session(db, params: list, node_id: Union[int, None], created_at):
    if not params:
        return False, False, None

    total_up = sum(item.get("up", 0) for item in params)
    total_down = sum(item.get("down", 0) for item in params)

    dbnode = None
    if node_id is not None:
        dbnode = db.query(Node).filter(Node.id == node_id).with_for_update().first()
        if dbnode is None:
            logger.warning("Skipping node usage for missing node id %s", node_id)
            return False, False, None

    usage_query = db.query(NodeUsage).filter(
        and_(NodeUsage.node_id == node_id, NodeUsage.created_at == created_at)
    )
    usage_row = usage_query.with_for_update().first()
    if usage_row is None:
        usage_row = NodeUsage(created_at=created_at, node_id=node_id, uplink=0, downlink=0)
        db.add(usage_row)
        db.flush()

    usage_row.uplink = int(usage_row.uplink or 0) + total_up
    usage_row.downlink = int(usage_row.downlink or 0) + total_down

    if dbnode is not None and (total_up or total_down):
        return _update_node_limits(db, dbnode, total_up, total_down, commit=False)

    return False, False, None


def _persist_node_usage_batch(api_params: dict, total_up: int, total_down: int):
    max_retries = max(int(JOB_USAGE_DB_MAX_RETRIES or 1), 1)
    tries = 0
    while True:
        created_at = hour_bucket()
        status_events = []
        try:
            with GetDB() as db:
                _set_usage_lock_wait_timeout(db)
                stmt = update(System).values(uplink=System.uplink + total_up, downlink=System.downlink + total_down)
                db.execute(stmt)

                _persist_outbound_traffic(db, api_params)

                if not DISABLE_RECORDING_NODE_USAGE:
                    for node_id, params in api_params.items():
                        limited, cleared, payload = _persist_node_stats_in_session(db, params, node_id, created_at)
                        if limited or cleared or payload:
                            status_events.append((node_id, limited, cleared, payload))

                db.commit()

            return status_events
        except (OperationalError, SQLTimeoutError) as exc:
            tries += 1
            if not is_retryable_db_error(exc) or tries >= max_retries:
                raise
            logger.warning(
                "Retryable database error while recording node usage, retrying (%s/%s)...", tries, max_retries
            )
            retry_delay(tries)


def _dispatch_node_limit_events(status_events):
    for node_id, limited_triggered, limit_cleared, status_change_payload in status_events:
        del node_id, limited_triggered, limit_cleared
        if status_change_payload:
            node_resp, prev_status = status_change_payload
            report.node_status_change(node_resp, previous_status=prev_status)


def record_node_usages():
    if not _record_node_usages_lock.acquire(blocking=False):
        logger.warning("record_node_usages is already running; skipping overlapping run")
        return None
    try:
        return _record_node_usages_once()
    finally:
        _record_node_usages_lock.release()


def _record_node_usages_once():
    collectors = {}
    for node_id, node in list(_nodes().items()):
        if getattr(node, "connected", False):
            collectors[node_id] = partial(_collect_node_outbound_stats, node)

    if not collectors:
        return None

    max_workers = max(1, min(int(JOB_RECORD_NODE_USAGE_WORKERS or 1), len(collectors)))
    timeout = max(int(JOB_RECORD_NODE_USAGE_COLLECT_TIMEOUT or 1), 1)
    executor = ThreadPoolExecutor(max_workers=max_workers)
    futures = {executor.submit(collector): node_id for node_id, collector in collectors.items()}
    done, pending = wait(futures.keys(), timeout=timeout)
    api_params = {}
    node_batches = {}
    for future in done:
        node_id = futures[future]
        try:
            result = future.result()
            node_batch_id = ""
            if isinstance(result, dict):
                node_batch_id = result.get("node_batch_id") or ""
                node_batches[node_id] = node_batch_id
                result = result.get("stats") or []
            if node_batch_id:
                api_params[node_id] = usage_delivery_buffer.replace_outbound_stats(node_id, result, node_batch_id)
            else:
                api_params[node_id] = usage_delivery_buffer.add_outbound_stats(node_id, result)
        except Exception as exc:  # pragma: no cover - defensive
            logger.warning("Failed to get outbound stats from node %s: %s", node_id, exc)
            api_params[node_id] = usage_delivery_buffer.pending_outbound_stats(node_id)

    for future in pending:
        node_id = futures[future]
        logger.warning("Timed out getting outbound stats from node %s after %ss", node_id, timeout)
        api_params[node_id] = usage_delivery_buffer.pending_outbound_stats(node_id)
        with contextlib.suppress(Exception):
            future.cancel()

    try:
        executor.shutdown(wait=False, cancel_futures=True)
    except TypeError:
        executor.shutdown(wait=False)

    total_up = 0
    total_down = 0
    for params in api_params.values():
        for param in params:
            total_up += param["up"]
            total_down += param["down"]

    if not (total_up or total_down):
        usage_delivery_buffer.ack_outbound_stats_for(api_params.keys(), node_batches)
        _ack_node_outbound_batches(node_batches)
        return None

    try:
        status_events = _persist_node_usage_batch(api_params, total_up, total_down)
    except (OperationalError, SQLTimeoutError) as exc:
        if is_retryable_db_error(exc):
            sample_count = sum(len(params or []) for params in api_params.values())
            logger.warning(
                "Deferred node usage write after retryable database error; %s samples remain in memory: %s",
                sample_count,
                exc,
            )
            return None
        raise
    usage_delivery_buffer.ack_outbound_stats_for(api_params.keys(), node_batches)
    _ack_node_outbound_batches(node_batches)
    _dispatch_node_limit_events(status_events)
    return None


__all__ = [
    "record_node_stats",
    "record_node_usages",
    "_record_node_usages_lock",
    "_record_node_usages_once",
    "_persist_node_usage_batch",
    "_persist_node_stats_in_session",
]
