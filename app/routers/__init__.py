from fastapi import APIRouter, Depends
from app.utils.request_context import capture_subscription_request_origin
from . import (
    ads,
    admin,
    runtime,
    node,
    subscription,
    subscription_alias,
    system,
    user_template,
    user,
    home,
    service,
    settings,
    myaccount,
)

api_router = APIRouter()

routers = [
    ads.router,
    admin.router,
    runtime.router,
    node.router,
    subscription.router,
    system.router,
    user_template.router,
    user.router,
    home.router,
    service.router,
    settings.router,
    myaccount.router,
    subscription_alias.router,
]

for router in routers:
    if router is runtime.router:
        api_router.include_router(router)
    else:
        api_router.include_router(router, dependencies=[Depends(capture_subscription_request_origin)])

__all__ = ["api_router"]
