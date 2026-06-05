def test_runtime_websocket_routes_are_not_wrapped_by_http_request_origin_dependency():
    from app.routers import api_router
    from app.utils.request_context import capture_subscription_request_origin

    websocket_routes = [
        route for route in api_router.routes if getattr(route, "path", "") in {"/api/core/logs", "/api/core/access/logs/ws"}
    ]
    assert websocket_routes
    for route in websocket_routes:
        dependency_calls = [dependency.dependency for dependency in getattr(route, "dependencies", [])]
        assert capture_subscription_request_origin not in dependency_calls
