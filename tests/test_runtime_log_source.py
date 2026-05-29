from types import SimpleNamespace

import pytest

from app.routers import runtime


def test_select_runtime_log_source_uses_connected_node(monkeypatch):
    node = SimpleNamespace(connected=True)
    monkeypatch.setattr(runtime, "xray", SimpleNamespace(nodes={7: node}))

    assert runtime._select_runtime_log_source() is node


def test_select_runtime_log_source_rejects_disconnected_node(monkeypatch):
    monkeypatch.setattr(runtime, "xray", SimpleNamespace(nodes={7: SimpleNamespace(connected=False)}))

    with pytest.raises(ValueError, match="not connected"):
        runtime._select_runtime_log_source("7")


def test_select_runtime_log_source_requires_node(monkeypatch):
    monkeypatch.setattr(runtime, "xray", SimpleNamespace(nodes={}))

    with pytest.raises(ValueError, match="No connected node"):
        runtime._select_runtime_log_source()
