import time
import traceback

from app.db import GetDB, crud
from app.models.node import NodeStatus
from app.runtime import logger, scheduler, xray
from app.utils.xray_targets import get_node_runtime_config
from config import NODE_RUNTIME_HEALTH_CHECK_INTERVAL


_AUTO_RECONNECT_COOLDOWN_SECONDS = max(30, NODE_RUNTIME_HEALTH_CHECK_INTERVAL * 3)
_last_auto_reconnect_attempt: dict[int, float] = {}


def _load_node_status_map() -> dict[int, NodeStatus]:
    try:
        with GetDB() as db:
            return {int(dbnode.id): dbnode.status for dbnode in crud.get_nodes(db) if dbnode.id is not None}
    except Exception:
        return {}


def _node_startup_config(node_id: int):
    with GetDB() as db:
        master_raw_config = crud.get_xray_config(db)
        dbnode = crud.get_node_by_id(db, node_id)
        if not dbnode:
            return xray.config.include_db_users()
        return get_node_runtime_config(
            db,
            dbnode,
            api_port=xray.config.api_port,
            master_config=master_raw_config,
        ).include_db_users()


def node_runtime_health_check():
    now = time.time()
    node_status_map = _load_node_status_map()

    for node_id, node in list(xray.nodes.items()):
        connected, started = node.refresh_health(force=True)
        dbnode_status = node_status_map.get(node_id)
        health_error = None

        if dbnode_status == NodeStatus.connected and not connected:
            health_error = "Health check failed: node is disconnected"
        elif dbnode_status == NodeStatus.connected and connected and not started:
            health_error = "Health check failed: node runtime is not started"

        if health_error:
            try:
                xray.operations.register_node_runtime_error(node_id, health_error)
            except Exception:
                pass

        if dbnode_status in (NodeStatus.disabled, NodeStatus.limited):
            _last_auto_reconnect_attempt.pop(node_id, None)
            continue

        if connected and started:
            _last_auto_reconnect_attempt.pop(node_id, None)
            try:
                with GetDB() as db:
                    dbnode = crud.get_node_by_id(db, node_id)
                    if dbnode and dbnode.status not in (NodeStatus.disabled, NodeStatus.limited):
                        if dbnode.status != NodeStatus.connected:
                            crud.update_node_status(
                                db,
                                dbnode,
                                NodeStatus.connected,
                                version=dbnode.xray_version,
                            )
            except Exception:
                pass
            continue

        last_attempt = _last_auto_reconnect_attempt.get(node_id, 0)
        if now - last_attempt < _AUTO_RECONNECT_COOLDOWN_SECONDS:
            continue

        _last_auto_reconnect_attempt[node_id] = now
        try:
            node_config = _node_startup_config(node_id)
            if connected:
                xray.operations.restart_node(node_id, node_config)
            else:
                xray.operations.connect_node(node_id, node_config, force=True)
        except Exception as exc:
            logger.warning("Failed to recover node %s during health check: %s", node_id, exc)


def start_node_runtime():
    logger.info("Preparing node runtime configs")

    start_time = time.time()
    try:
        xray.config.include_db_users()
        logger.info("Node runtime config prepared in %.2f seconds", time.time() - start_time)
    except Exception as e:
        logger.error("Failed to generate default Xray config: %s", e)
        traceback.print_exc()
        return

    logger.info("Connecting enabled nodes")
    try:
        with GetDB() as db:
            master_raw_config = crud.get_xray_config(db)
            dbnodes = crud.get_nodes(db=db, enabled=True)
            if not dbnodes:
                logger.warning("No enabled Xray nodes found; add a node before serving users.")
            node_configs = {
                dbnode.id: get_node_runtime_config(
                    db,
                    dbnode,
                    api_port=xray.config.api_port,
                    master_config=master_raw_config,
                ).include_db_users()
                for dbnode in dbnodes
            }
            for dbnode in dbnodes:
                crud.update_node_status(db, dbnode, NodeStatus.connecting)

        for node_id, node_config in node_configs.items():
            try:
                xray.operations.connect_node(node_id, node_config)
            except Exception as e:
                logger.error("Failed to connect to node %s: %s", node_id, e)
                traceback.print_exc()
    except Exception as e:
        logger.error("Failed to start nodes: %s", e)
        traceback.print_exc()

    scheduler.add_job(
        node_runtime_health_check,
        "interval",
        seconds=NODE_RUNTIME_HEALTH_CHECK_INTERVAL,
        coalesce=True,
        max_instances=1,
        id="node_runtime_health_check",
        replace_existing=True,
    )


def shutdown_node_runtime():
    logger.info("Disconnecting node runtimes")
    for node in list(xray.nodes.values()):
        try:
            node.disconnect()
        except Exception:
            pass
