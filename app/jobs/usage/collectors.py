"""Compatibility helpers for stats sources exposed by nodes.

The master does not start or own an Xray core anymore. These helpers only
operate on an explicit stats source passed by a node object, preserving the
buffered node path as the default when no direct stats API is available.
"""

from collections import defaultdict


def resolve_stats_api(source):
    api = getattr(source, "api", None)
    if api is not None:
        return api
    if hasattr(source, "get_users_stats") or hasattr(source, "get_outbounds_stats"):
        return source
    if hasattr(source, "query_stats"):
        return source
    return None


def _call_stats(api, method_name: str, pattern: str):
    method = getattr(api, method_name, None)
    if callable(method):
        return method(reset=True, timeout=10) or []
    query_stats = getattr(api, "query_stats", None)
    if callable(query_stats):
        return query_stats(pattern, reset=True, timeout=10) or []
    return []


def get_users_stats(api=None):
    if api is None:
        return []

    totals: dict[str, int] = defaultdict(int)
    for stat in _call_stats(api, "get_users_stats", "user>>>"):
        if getattr(stat, "type", None) != "user":
            continue
        name = str(getattr(stat, "name", "") or "")
        uid = name.split(".", 1)[0].strip()
        if not uid:
            continue
        totals[uid] += int(getattr(stat, "value", 0) or 0)

    return [{"uid": uid, "value": value} for uid, value in totals.items() if value]


def get_outbounds_stats(api=None):
    if api is None:
        return []

    totals: dict[str, dict[str, int]] = defaultdict(lambda: {"up": 0, "down": 0})
    for stat in _call_stats(api, "get_outbounds_stats", "outbound>>>"):
        if getattr(stat, "type", None) != "outbound":
            continue
        tag = str(getattr(stat, "name", "") or "")
        if not tag or tag == "api":
            continue
        link = str(getattr(stat, "link", "") or "")
        value = int(getattr(stat, "value", 0) or 0)
        if link == "uplink":
            totals[tag]["up"] += value
        elif link == "downlink":
            totals[tag]["down"] += value

    return [
        {"tag": tag, "up": values["up"], "down": values["down"]}
        for tag, values in totals.items()
        if values["up"] or values["down"]
    ]
