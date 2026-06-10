from __future__ import annotations

from typing import Any, Dict, Optional

from sqlalchemy.orm import Session

from app.db import GetDB, crud
from app.db import models as db_models


def get_xray_config_cached(db: Session, force_refresh: bool = False) -> dict:
    # TODO(go-config-cleanup): keep only until remaining Python validators no
    # longer need to inspect stored Xray JSON.
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


def _stream_metadata(inbound: dict) -> dict:
    stream = inbound.get("streamSettings") or {}
    if not isinstance(stream, dict):
        stream = {}
    network = stream.get("network") or "tcp"
    network_settings = stream.get(f"{network}Settings") or {}
    if not isinstance(network_settings, dict):
        network_settings = {}
    headers = network_settings.get("headers") or {}
    if not isinstance(headers, dict):
        headers = {}
    tls_settings = stream.get("tlsSettings") or stream.get("realitySettings") or stream.get("xtlsSettings") or {}
    if not isinstance(tls_settings, dict):
        tls_settings = {}
    return {
        "network": network,
        "tls": stream.get("security"),
        "path": network_settings.get("path") or "",
        "host": headers.get("Host", []),
        "sni": tls_settings.get("serverName") or "",
    }


def _inbounds_from_raw_config(raw_config: dict) -> Dict[str, Any]:
    inbounds: Dict[str, Any] = {}
    for inbound in raw_config.get("inbounds") or []:
        if not isinstance(inbound, dict):
            continue
        tag = inbound.get("tag")
        protocol = inbound.get("protocol")
        if not tag or not protocol:
            continue
        item = dict(inbound)
        item.setdefault("tag", tag)
        item.setdefault("protocol", protocol)
        item.setdefault("port", inbound.get("port"))
        item.update({key: value for key, value in _stream_metadata(inbound).items() if key not in item})
        inbounds.setdefault(str(tag), item)
    return inbounds


def get_inbounds_by_tag_cached(db: Session, force_refresh: bool = False) -> Dict[str, Any]:
    # Active inbound management is Go-native. This lightweight reader is kept
    # only for transitional Python validators/serializers that need tag and
    # protocol metadata from stored JSON.
    del force_refresh
    from app.utils.xray_targets import iter_stored_raw_configs

    inbounds: Dict[str, Any] = {}
    try:
        config_sources = list(iter_stored_raw_configs(db))
    except Exception:
        config_sources = []
    for _target_id, raw_config in config_sources:
        if not isinstance(raw_config, dict):
            continue
        for tag, inbound in _inbounds_from_raw_config(raw_config).items():
            inbounds.setdefault(tag, inbound)
    return inbounds
