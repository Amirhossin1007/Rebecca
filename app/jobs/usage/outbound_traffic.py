"""
Legacy compatibility module.

Outbound usage collection is part of the Go/gRPC node usage collector. This
module remains only for imports from older tests or integrations.
"""

from app.jobs.usage.go_collector import record_node_usages


def record_outbound_traffic_from_params(*args, **kwargs):
    del args, kwargs
    return None


def record_outbound_traffic(*args, **kwargs):
    del args, kwargs
    return record_node_usages()
