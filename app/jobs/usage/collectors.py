"""Compatibility shims for the old direct Xray stats collectors.

The master no longer talks to any Xray Stats API directly. Usage samples are
collected and buffered by Rebecca-node, then delivered to the master over the
node control API.
"""


def resolve_stats_api(_source):
    return None


def get_users_stats(_api=None):
    return []


def get_outbounds_stats(_api=None):
    return []
