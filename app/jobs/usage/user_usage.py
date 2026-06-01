"""
Legacy compatibility module.

User usage collection is handled by the Go/gRPC collector in
app.jobs.usage.go_collector. This file intentionally does not import the
Python node runtime.
"""

from app.jobs.usage.go_collector import record_user_usages


def _collect_user_stats(*args, **kwargs):
    del args, kwargs
    return []


def record_user_stats(*args, **kwargs):
    del args, kwargs
    return record_user_usages()
