from __future__ import annotations

import argparse
import hashlib
import json
from datetime import UTC, datetime
from pathlib import Path
from typing import Any


def _utcnow() -> str:
    return datetime.now(UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def _load_manifest(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}
    return data if isinstance(data, dict) else {}


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _platform_from_asset(name: str, build_tag: str) -> str | None:
    suffixes = [f"-{build_tag}.tar.gz", f"-{build_tag}.zip"]
    base = name
    for suffix in suffixes:
        if base.endswith(suffix):
            base = base[: -len(suffix)]
            break

    if base.startswith("rebecca-node-"):
        platform = base.removeprefix("rebecca-node-")
    elif base.startswith("rebecca-"):
        platform = base.removeprefix("rebecca-")
    else:
        return None

    return platform or None


def _asset_url(repo: str, release_tag: str, asset_name: str) -> str:
    return f"https://github.com/{repo}/releases/download/{release_tag}/{asset_name}"


def _build_assets(assets_dir: Path, *, repo: str, release_tag: str, build_tag: str) -> dict[str, dict[str, Any]]:
    assets: dict[str, dict[str, Any]] = {}
    for path in sorted(assets_dir.iterdir()):
        if not path.is_file():
            continue
        platform = _platform_from_asset(path.name, build_tag)
        if not platform:
            continue
        assets[platform] = {
            "name": path.name,
            "url": _asset_url(repo, release_tag, path.name),
            "sha256": _sha256(path),
            "size": path.stat().st_size,
        }
    return assets


def main() -> int:
    parser = argparse.ArgumentParser(description="Update Rebecca dev binary build manifest.")
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--assets-dir", required=True)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--release-tag", default="dev-builds")
    parser.add_argument("--project", default="Rebecca")
    parser.add_argument("--build-tag", required=True)
    parser.add_argument("--sha", required=True)
    parser.add_argument("--branch", default="dev")
    parser.add_argument("--workflow", default="")
    parser.add_argument("--run-id", default="")
    parser.add_argument("--run-number", default="")
    parser.add_argument("--max-builds", type=int, default=50)
    args = parser.parse_args()

    manifest_path = Path(args.manifest)
    assets_dir = Path(args.assets_dir)
    now = _utcnow()

    manifest = _load_manifest(manifest_path)
    builds = manifest.get("builds")
    if not isinstance(builds, list):
        builds = []

    assets = _build_assets(
        assets_dir,
        repo=args.repo,
        release_tag=args.release_tag,
        build_tag=args.build_tag,
    )
    if not assets:
        raise SystemExit(f"No manifest-compatible assets found in {assets_dir}")

    build = {
        "tag": args.build_tag,
        "sha": args.sha,
        "short_sha": args.sha[:7],
        "branch": args.branch,
        "workflow": args.workflow,
        "run_id": args.run_id,
        "run_number": args.run_number,
        "created_at": now,
        "assets": assets,
    }

    builds = [item for item in builds if not isinstance(item, dict) or item.get("tag") != args.build_tag]
    builds.insert(0, build)
    max_builds = max(args.max_builds, 1)

    manifest = {
        "schema": 1,
        "project": args.project,
        "channel": "dev",
        "latest": args.build_tag,
        "updated_at": now,
        "release_tag": args.release_tag,
        "builds": builds[:max_builds],
    }
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
