from config import JOB_RECORD_NODE_USAGES_INTERVAL, JOB_RECORD_USER_USAGES_INTERVAL

from app.runtime import scheduler

from .go_collector import record_node_usages, record_user_usages


"""Public interface for usage jobs: exports record functions and registers scheduler tasks."""

# region Scheduler registration


def register_usage_jobs():
    if not scheduler:
        return

    scheduler.add_job(
        record_user_usages,
        "interval",
        seconds=JOB_RECORD_USER_USAGES_INTERVAL,
        coalesce=True,
        max_instances=1,
        id="record_user_usages",
        replace_existing=True,
    )
    scheduler.add_job(
        record_node_usages,
        "interval",
        seconds=JOB_RECORD_NODE_USAGES_INTERVAL,
        coalesce=True,
        max_instances=1,
        id="record_node_usages",
        replace_existing=True,
    )


# endregion

# region Public exports


def record_node_stats(*args, **kwargs):
    del args, kwargs
    return record_node_usages()


def record_user_stats(*args, **kwargs):
    del args, kwargs
    return record_user_usages()


def record_outbound_traffic(*args, **kwargs):
    del args, kwargs
    return record_node_usages()

__all__ = [
    "record_node_stats",
    "record_node_usages",
    "record_outbound_traffic",
    "record_user_stats",
    "record_user_usages",
    "register_usage_jobs",
]

# endregion
