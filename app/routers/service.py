from fastapi import APIRouter, Depends, HTTPException

from app.models.admin import Admin
from app.utils import responses

router = APIRouter(
    prefix="/api/v2/services",
    tags=["Service V2"],
    responses={401: responses._401},
)

NATIVE_SERVICE_API_REQUIRED_DETAIL = (
    "This service route is served directly by the Go gateway and Go Master API."
)


def _native_service_route_disabled() -> None:
    raise HTTPException(status_code=503, detail=NATIVE_SERVICE_API_REQUIRED_DETAIL)


@router.get("")
def get_services(_: Admin = Depends(Admin.get_current)):
    _native_service_route_disabled()


@router.post("")
def create_service(_: Admin = Depends(Admin.check_sudo_admin)):
    _native_service_route_disabled()


@router.put("/{service_id}/admins/{admin_id}/limits")
def update_service_admin_limits(
    service_id: int,
    admin_id: int,
    _: Admin = Depends(Admin.check_sudo_admin),
):
    del service_id, admin_id
    _native_service_route_disabled()


@router.get("/{service_id}")
def get_service_detail(service_id: int, _: Admin = Depends(Admin.get_current)):
    del service_id
    _native_service_route_disabled()


@router.put("/{service_id}")
def modify_service(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.post("/{service_id}/auto-inbound", responses={403: responses._403, 404: responses._404})
def create_service_auto_inbound(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.delete("/{service_id}/auto-inbound", responses={403: responses._403, 404: responses._404})
def delete_service_auto_inbound(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.delete("/{service_id}", status_code=204)
def delete_service(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.post("/{service_id}/reset-usage")
def reset_service_usage(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.get("/{service_id}/usage/timeseries")
def get_service_usage_timeseries(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.get("/{service_id}/usage/admins")
def get_service_usage_by_admin(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.get("/{service_id}/usage/admin-timeseries")
def get_service_admin_usage_timeseries(service_id: int, _: Admin = Depends(Admin.check_sudo_admin)):
    del service_id
    _native_service_route_disabled()


@router.get("/{service_id}/users")
def get_service_users(service_id: int, _: Admin = Depends(Admin.get_current)):
    del service_id
    _native_service_route_disabled()


@router.post("/{service_id}/users/actions", responses={403: responses._403})
def perform_service_users_action(service_id: int, _: Admin = Depends(Admin.get_current)):
    del service_id
    _native_service_route_disabled()
