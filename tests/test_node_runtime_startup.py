from types import SimpleNamespace

import app.runtime

app.runtime.scheduler = SimpleNamespace(add_job=lambda *args, **kwargs: None)

from app.jobs import node_runtime


def test_start_node_runtime_registers_health_check_with_stable_id(monkeypatch):
    calls = []

    class DummyScheduler:
        def add_job(self, *args, **kwargs):
            calls.append((args, kwargs))

    class DummyDB:
        def __enter__(self):
            return object()

        def __exit__(self, exc_type, exc_value, traceback):
            return False

    class DummyConfig:
        api_port = 10085

        def include_db_users(self):
            return {"inbounds": [], "outbounds": []}

    fake_xray = SimpleNamespace(
        config=DummyConfig(),
        nodes={},
        operations=SimpleNamespace(connect_node=lambda *args, **kwargs: None),
    )

    monkeypatch.setattr(node_runtime, "scheduler", DummyScheduler())
    monkeypatch.setattr(node_runtime, "xray", fake_xray)
    monkeypatch.setattr(node_runtime, "GetDB", DummyDB)
    monkeypatch.setattr(node_runtime.crud, "get_xray_config", lambda db: {})
    monkeypatch.setattr(node_runtime.crud, "get_nodes", lambda db=None, enabled=None: [])

    node_runtime.start_node_runtime()

    assert len(calls) == 1
    _, kwargs = calls[0]
    assert kwargs["id"] == "node_runtime_health_check"
    assert kwargs["replace_existing"] is True
    assert kwargs["max_instances"] == 1
