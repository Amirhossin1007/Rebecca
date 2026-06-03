"""
Legacy compatibility module.

Outbound usage collection is part of the Go/gRPC node usage collector. This
module remains only for imports from older tests or integrations.
"""

import logging

from app.db import GetDB, crud
from app.db.models import Node, OutboundTraffic
from app.jobs.usage.go_collector import record_node_usages
from app.utils.outbound import extract_outbound_metadata, generate_outbound_id
from app.utils.xray_targets import MASTER_TARGET_ID, get_node_effective_raw_config, node_target_id


logger = logging.getLogger(__name__)


class _RuntimeXrayProxy:
    """Live compatibility target for legacy outbound metadata tests."""

    def __getattr__(self, name):
        from app import runtime as runtime_state

        target = runtime_state.xray
        if target is None:
            raise AttributeError(name)
        return getattr(target, name)


xray = _RuntimeXrayProxy()


def _target_identity(node_id: int | None) -> tuple[str, int | None]:
    if node_id is None:
        return MASTER_TARGET_ID, None
    return node_target_id(node_id), node_id


def _load_outbound_configs(db, node_id: int | None) -> dict[str, dict]:
    try:
        config = crud.get_xray_config(db) or {}
        if node_id is None:
            outbounds_config = config.get("outbounds", []) if isinstance(config, dict) else []
        else:
            master_config = config.copy() if isinstance(config, dict) else {}
            dbnode = db.query(Node).filter_by(id=node_id).first()
            target_config = get_node_effective_raw_config(dbnode, master_config) if dbnode else master_config
            outbounds_config = target_config.get("outbounds", [])
    except Exception:
        logger.warning("Failed to get outbound configs for target %s", node_id or "master")
        outbounds_config = []

    return {outbound.get("tag", ""): outbound for outbound in outbounds_config if isinstance(outbound, dict)}


def _aggregate_by_tag(params: list[dict] | None) -> dict[str, dict]:
    stats_by_tag: dict[str, dict] = {}
    for stat in params or []:
        tag = str(stat.get("tag") or "").strip()
        if not tag:
            continue
        item = stats_by_tag.setdefault(tag, {"up": 0, "down": 0, "tag": tag})
        item["up"] += int(stat.get("up") or 0)
        item["down"] += int(stat.get("down") or 0)
    return stats_by_tag


def _apply_metadata(record: OutboundTraffic, outbound_config: dict | None, tag: str) -> None:
    if not outbound_config:
        if not record.tag:
            record.tag = tag
        return

    metadata = extract_outbound_metadata(outbound_config)
    record.tag = metadata.get("tag") or tag
    if metadata.get("protocol") is not None:
        record.protocol = metadata["protocol"]
    if metadata.get("address") is not None:
        record.address = metadata["address"]
    if metadata.get("port") is not None:
        record.port = metadata["port"]


def _persist_outbound_traffic(db, api_params: dict) -> tuple[int, int, int]:
    if not any(api_params.values()):
        return 0, 0, 0

    records_updated = 0
    records_created = 0
    stats_count = 0

    for node_id, params in api_params.items():
        stats_by_tag = _aggregate_by_tag(params)
        if not stats_by_tag:
            continue
        stats_count += len(stats_by_tag)
        target_id, db_node_id = _target_identity(node_id)
        outbounds_by_tag = _load_outbound_configs(db, node_id)

        for tag, stat in stats_by_tag.items():
            up = int(stat.get("up") or 0)
            down = int(stat.get("down") or 0)
            if not (up or down):
                continue

            outbound_config = outbounds_by_tag.get(tag)
            outbound_id = generate_outbound_id(outbound_config) if outbound_config else f"tag_{tag}"
            existing = (
                db.query(OutboundTraffic)
                .filter(OutboundTraffic.target_id == target_id, OutboundTraffic.outbound_id == outbound_id)
                .first()
            )
            if existing is None:
                existing = (
                    db.query(OutboundTraffic)
                    .filter(OutboundTraffic.target_id == target_id, OutboundTraffic.tag == tag)
                    .first()
                )

            if existing:
                existing.target_id = target_id
                existing.node_id = db_node_id
                existing.uplink = int(existing.uplink or 0) + up
                existing.downlink = int(existing.downlink or 0) + down
                existing.outbound_id = outbound_id
                _apply_metadata(existing, outbound_config, tag)
                records_updated += 1
                continue

            metadata = extract_outbound_metadata(outbound_config) if outbound_config else {}
            db.add(
                OutboundTraffic(
                    target_id=target_id,
                    node_id=db_node_id,
                    outbound_id=outbound_id,
                    tag=metadata.get("tag") or tag,
                    protocol=metadata.get("protocol"),
                    address=metadata.get("address"),
                    port=metadata.get("port"),
                    uplink=up,
                    downlink=down,
                )
            )
            records_created += 1

    return records_updated, records_created, stats_count


def record_outbound_traffic_from_params(*args, **kwargs):
    if args and isinstance(args[0], dict):
        with GetDB() as db:
            _persist_outbound_traffic(db, args[0])
            db.commit()
        return None
    del args, kwargs
    return None


def record_outbound_traffic(*args, **kwargs):
    del args, kwargs
    return record_node_usages()
