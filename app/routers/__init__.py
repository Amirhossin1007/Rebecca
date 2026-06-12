from fastapi import APIRouter

api_router = APIRouter()

routers = []

for router in routers:
    api_router.include_router(router)

__all__ = ["api_router"]
