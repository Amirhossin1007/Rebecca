from fastapi import APIRouter, HTTPException, WebSocket

from app.db import GetDB
from app.models.admin import Admin, AdminRole
from app.utils import responses

router = APIRouter(tags=["Node"], prefix="/api", responses={401: responses._401, 403: responses._403})

MASTER_NODE_ROUTE_GONE_DETAIL = "Master node usage/runtime routes have been removed."
NODE_ROUTE_DISABLED_DETAIL = "This node route is served directly by the Go gateway and Go Master API."


def _master_node_route_gone() -> None:
    # TODO: remove master_node_state schema and CRUD in a dedicated DB cleanup migration.
    raise HTTPException(status_code=410, detail=MASTER_NODE_ROUTE_GONE_DETAIL)


def _node_route_disabled() -> None:
    raise HTTPException(status_code=503, detail=NODE_ROUTE_DISABLED_DETAIL)


@router.get("/node/master")
def get_master_node_state():
    _master_node_route_gone()


@router.put("/node/master")
def update_master_node_state():
    _master_node_route_gone()


@router.post("/node/master/usage/reset")
def reset_master_node_usage():
    _master_node_route_gone()


@router.get("/node/settings")
def get_node_settings():
    _node_route_disabled()


@router.post("/node/certificate/new")
def issue_node_certificate():
    _node_route_disabled()


@router.post("/node")
def add_node():
    _node_route_disabled()


@router.get("/node/{node_id}")
def get_node(node_id: int):
    _node_route_disabled()


@router.put("/node/{node_id}")
def modify_node(node_id: int):
    _node_route_disabled()


@router.delete("/node/{node_id}")
def remove_node(node_id: int):
    _node_route_disabled()


@router.get("/node/{node_id}/logs")
def get_node_logs(node_id: int, max_lines: int = 200):
    _node_route_disabled()


@router.websocket("/node/{node_id}/logs")
async def node_logs(node_id: int, websocket: WebSocket):
    # TODO: replace this placeholder with Go gateway WebSocket/streaming log support.
    token = websocket.query_params.get("token") or websocket.headers.get("Authorization", "").removeprefix("Bearer ")
    with GetDB() as db:
        admin = Admin.get_admin(token, db)
    if not admin:
        return await websocket.close(reason="Unauthorized", code=4401)

    if admin.role not in (AdminRole.sudo, AdminRole.full_access):
        return await websocket.close(reason="You're not allowed", code=4403)

    return await websocket.close(reason=NODE_ROUTE_DISABLED_DETAIL, code=4400)


@router.get("/nodes")
def get_nodes():
    _node_route_disabled()


@router.get("/nodes/usage")
def get_usage(start: str = "", end: str = ""):
    _node_route_disabled()


@router.post("/node/{node_id}/certificate/regenerate")
def node_certificate_regenerate_disabled(node_id: int):
    _node_route_disabled()


@router.post("/node/{node_id}/usage/reset")
def node_usage_reset_disabled(node_id: int):
    _node_route_disabled()


@router.post("/node/{node_id}/reconnect")
def reconnect_node(node_id: int):
    _node_route_disabled()


@router.post("/node/{node_id}/restart")
def restart_node_runtime(node_id: int):
    _node_route_disabled()


@router.post("/node/{node_id}/sync")
def sync_node(node_id: int):
    _node_route_disabled()


@router.get("/node/{node_id}/usage/daily", responses={403: responses._403, 404: responses._404})
def get_node_usage_daily(node_id: int, start: str = "", end: str = "", granularity: str = "day"):
    _node_route_disabled()


@router.post("/node/{node_id}/xray/update", responses={403: responses._403, 404: responses._404})
def update_node_runtime(node_id: int):
    _node_route_disabled()


@router.post("/node/{node_id}/geo/update", responses={403: responses._403, 404: responses._404})
def update_node_geo(node_id: int):
    _node_route_disabled()


@router.post("/node/{node_id}/service/restart", responses={403: responses._403, 404: responses._404})
def restart_node_service(node_id: int):
    _node_route_disabled()


@router.post("/node/{node_id}/service/update", responses={403: responses._403, 404: responses._404})
def update_node_service(node_id: int):
    _node_route_disabled()
