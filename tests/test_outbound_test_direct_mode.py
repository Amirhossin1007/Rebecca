from types import SimpleNamespace

from app.routers import runtime


def test_run_outbound_ping_test_requires_node(monkeypatch):
    monkeypatch.setattr(runtime, "xray", SimpleNamespace(nodes={}))

    result = runtime._run_outbound_ping_test(
        outbound_tag="DIRECT",
        all_outbounds=[],
        outbound_protocol="freedom",
    )

    assert result["success"] is False
    assert "No connected node" in result["error"]


def test_run_outbound_ping_test_uses_node(monkeypatch):
    class FakeNode:
        connected = True

        def test_outbound(self, **kwargs):
            assert kwargs["outbound_tag"] == "DIRECT"
            return {"success": True, "delay": 7, "statusCode": 204}

    monkeypatch.setattr(runtime, "xray", SimpleNamespace(nodes={1: FakeNode()}))

    result = runtime._run_outbound_ping_test(
        outbound_tag="DIRECT",
        all_outbounds=[],
        outbound_protocol="freedom",
    )

    assert result["success"] is True
    assert result["delay"] == 7
