from __future__ import annotations

import os
from typing import Any, Dict, Optional

from sqlalchemy.orm import Session

from app.db import GetDB, crud
from app.db import models as db_models


def get_xray_config_cached(db: Session, force_refresh: bool = False) -> dict:
    del force_refresh
    return crud.get_xray_config(db)


def get_service_allowed_inbounds_cached(db: Session, service) -> Dict[str, Any]:
    return crud.get_service_allowed_inbounds(service)


def _host_to_dict(host, service_ids: list[int]) -> dict:
    return {
        "remark": host.remark,
        "address": [i.strip() for i in host.address.split(",")] if host.address else [],
        "port": host.port,
        "path": host.path if host.path else None,
        "sni": [i.strip() for i in host.sni.split(",")] if host.sni else [],
        "host": [i.strip() for i in host.host.split(",")] if host.host else [],
        "alpn": host.alpn.value,
        "fingerprint": host.fingerprint.value,
        "tls": None if host.security.name == "inbound_default" else host.security.value,
        "allowinsecure": host.allowinsecure,
        "mux_enable": host.mux_enable,
        "fragment_setting": host.fragment_setting,
        "noise_setting": host.noise_setting,
        "random_user_agent": host.random_user_agent,
        "use_sni_as_host": host.use_sni_as_host,
        "sort": host.sort if host.sort is not None else 0,
        "id": host.id,
        "service_ids": service_ids,
        "is_disabled": host.is_disabled,
        "inbound_tag": host.inbound_tag,
    }


def get_service_host_map_cached(service_id: Optional[int], force_refresh: bool = False) -> Dict[str, Any]:
    del force_refresh
    with GetDB() as db:
        inbound_tags = set(get_inbounds_by_tag_cached(db).keys())
        host_map: Dict[str, list] = {tag: [] for tag in inbound_tags}
        hosts = db.query(db_models.ProxyHost).all()
        for host in hosts:
            if host.is_disabled or host.inbound_tag not in inbound_tags:
                continue
            service_ids = [link.service_id for link in getattr(host, "service_links", []) if link.service_id]
            if service_id is not None and service_id not in service_ids:
                continue
            host_map.setdefault(host.inbound_tag, []).append(_host_to_dict(host, service_ids))
        for tag in inbound_tags:
            host_map.setdefault(tag, [])
            host_map[tag].sort(key=lambda h: (h.get("sort", 0), h.get("id") or 0))
        return host_map


def _runtime_inbounds_by_tag() -> Dict[str, Any]:
    try:
        from app import runtime as runtime_state

        runtime_xray = runtime_state.xray
        runtime_config = getattr(runtime_xray, "config", None)
        runtime_inbounds = getattr(runtime_config, "inbounds_by_tag", None)
        runtime_protocols = getattr(runtime_config, "inbounds_by_protocol", None)
        inbounds: Dict[str, Any] = {}
        if isinstance(runtime_inbounds, dict):
            inbounds.update(
                {
                    tag: dict(inbound) if isinstance(inbound, dict) else {"tag": tag}
                    for tag, inbound in runtime_inbounds.items()
                }
            )
        if isinstance(runtime_protocols, dict):
            for protocol, protocol_inbounds in runtime_protocols.items():
                for inbound in protocol_inbounds or []:
                    if not isinstance(inbound, dict):
                        continue
                    tag = inbound.get("tag")
                    if not tag:
                        continue
                    item = inbounds.setdefault(tag, {"tag": tag})
                    item.setdefault("tag", tag)
                    item.setdefault("protocol", protocol)
        return inbounds
    except Exception:
        return {}


def get_inbounds_by_tag_cached(db: Session, force_refresh: bool = False) -> Dict[str, Any]:
    del force_refresh
    from app.utils.xray_targets import iter_stored_raw_configs
    from app.xray.config import XRayConfig

    inbounds: Dict[str, Any] = {}
    try:
        config_sources = list(iter_stored_raw_configs(db))
    except Exception:
        config_sources = []
    for _target_id, raw_config in config_sources:
        try:
            config = XRayConfig(raw_config, api_port=8080)
        except Exception:
            continue
        for tag, inbound in config.inbounds_by_tag.items():
            inbounds.setdefault(tag, inbound)
    runtime_inbounds = _runtime_inbounds_by_tag()
    if runtime_inbounds and (not inbounds or os.getenv("REBECCA_SKIP_RUNTIME_INIT") == "1"):
        inbounds.update(runtime_inbounds)
    return inbounds
