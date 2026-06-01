import threading

from app.runtime import logger, scheduler
from app.services import node_operations


_queue_lock = threading.Lock()


def process_node_operations():
    if not _queue_lock.acquire(blocking=False):
        return
    try:
        node_operations.process_pending_operations(limit=100)
    except Exception as exc:  # pragma: no cover - scheduler resilience
        logger.warning("Node operation queue processing failed: %s", exc)
    finally:
        _queue_lock.release()


scheduler.add_job(
    process_node_operations,
    "interval",
    seconds=15,
    coalesce=True,
    max_instances=1,
    misfire_grace_time=15,
    replace_existing=True,
)
