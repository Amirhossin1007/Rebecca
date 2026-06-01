from __future__ import annotations

from typing import Any

from app.services.go_usage import call_bridge


def _runtime_payload(node_id: int, **extra: Any) -> dict[str, Any]:
    payload: dict[str, Any] = {"node_id": int(node_id), **extra}
    return payload


def connect_node(node_id: int, *, force: bool = False) -> dict[str, Any]:
    return call_bridge("node.connect", _runtime_payload(node_id, force=force))


def reconnect_node(node_id: int) -> dict[str, Any]:
    return call_bridge("node.reconnect", _runtime_payload(node_id, force=True))


def restart_node(node_id: int) -> dict[str, Any]:
    return call_bridge("node.restart", _runtime_payload(node_id))


def sync_node(node_id: int) -> dict[str, Any]:
    return call_bridge("node.sync", _runtime_payload(node_id))


def update_runtime(node_id: int, *, version: str) -> dict[str, Any]:
    return call_bridge("node.runtime.update", _runtime_payload(node_id, version=version))


def update_geo(node_id: int, *, files: list[dict[str, Any]]) -> dict[str, Any]:
    return call_bridge("node.geo.update", _runtime_payload(node_id, files=files))


def restart_service(node_id: int) -> dict[str, Any]:
    return call_bridge("node.service.restart", _runtime_payload(node_id))


def update_service(node_id: int, *, channel: str | None = None, version: str | None = None) -> dict[str, Any]:
    payload: dict[str, Any] = {}
    if channel is not None:
        payload["channel"] = channel
    if version is not None:
        payload["version"] = version
    return call_bridge("node.service.update", _runtime_payload(node_id, **payload))


def list_nodes() -> list[dict[str, Any]]:
    result = call_bridge("node.list", {})
    return result if isinstance(result, list) else []


def get_node(node_id: int) -> dict[str, Any]:
    result = call_bridge("node.get", _runtime_payload(node_id))
    return result if isinstance(result, dict) else {}


def health(node_id: int) -> dict[str, Any]:
    return call_bridge("node.health", _runtime_payload(node_id))


def metrics(node_id: int) -> dict[str, Any]:
    return call_bridge("node.metrics", _runtime_payload(node_id))


def logs(node_id: int, *, max_lines: int = 200) -> list[str]:
    result = call_bridge("node.logs", _runtime_payload(node_id, max_lines=max_lines))
    if isinstance(result, dict):
        lines = result.get("logs") or []
        if isinstance(lines, list):
            return [str(line) for line in lines]
    return []
