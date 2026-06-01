"""
Legacy compatibility module.

Node usage collection is handled by the Go/gRPC collector in
app.jobs.usage.go_collector. This file intentionally does not import the
Python node runtime.
"""

from app.jobs.usage.go_collector import record_node_usages


def record_node_stats(*args, **kwargs):
    del args, kwargs
    return record_node_usages()
