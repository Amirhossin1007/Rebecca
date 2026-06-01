import logging

from app.db import GetDB, crud
from app.models.node import NodeStatus
from app.services import go_node, node_operations

logger = logging.getLogger("uvicorn.error")


def node_runtime_health_check():
    """Legacy scheduler hook kept as a no-op after the Go/gRPC node migration."""
    return None


def start_node_runtime():
    """
    Legacy startup hook.

    The master no longer starts or owns a Python node runtime. It only asks the
    Go controller to connect enabled nodes and queues a config sync so returning
    nodes converge to the database state.
    """
    logger.info("Bootstrapping Go node controller")
    try:
        node_operations.queue_sync_config()
    except Exception as exc:
        logger.warning("Failed to queue startup node sync: %s", exc, exc_info=True)

    try:
        with GetDB() as db:
            nodes = [
                dbnode
                for dbnode in crud.get_nodes(db=db, enabled=True)
                if dbnode.status not in (NodeStatus.disabled, NodeStatus.limited)
            ]
            for dbnode in nodes:
                crud.update_node_status(db, dbnode, NodeStatus.connecting)
            node_ids = [int(dbnode.id) for dbnode in nodes if dbnode.id is not None]
    except Exception as exc:
        logger.warning("Failed to load enabled nodes for Go bootstrap: %s", exc, exc_info=True)
        return

    for node_id in node_ids:
        try:
            go_node.connect_node(node_id)
        except Exception as exc:
            logger.warning("Go node bootstrap failed for node %s: %s", node_id, exc)


def shutdown_node_runtime():
    """Legacy shutdown hook kept for import compatibility."""
    return None
