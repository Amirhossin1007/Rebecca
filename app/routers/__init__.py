from fastapi import APIRouter, Depends
from app.utils.request_context import capture_subscription_request_origin
from . import (
    ads,
    runtime,
    node,
    system,
    user_template,
    home,
    service,
    settings,
)

api_router = APIRouter()

routers = [
    ads.router,
    runtime.router,
    node.router,
    system.router,
    user_template.router,
    home.router,
    service.router,
    settings.router,
]

for router in routers:
    if router is runtime.router:
        api_router.include_router(router)
    else:
        api_router.include_router(router, dependencies=[Depends(capture_subscription_request_origin)])

__all__ = ["api_router"]
