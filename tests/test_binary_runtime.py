import json
import os
import subprocess
import sys
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parents[1]
REBECCA_SCRIPT_PATH = PROJECT_ROOT / "scripts" / "rebecca" / "rebecca.sh"
REBECCA_CLI_PATH = PROJECT_ROOT / "rebecca-cli.py"


def test_config_loads_env_from_installed_binary_layout(tmp_path: Path):
    app_dir = tmp_path / "rebecca"
    bin_dir = app_dir / "bin"
    bin_dir.mkdir(parents=True)

    expected_url = f"sqlite:///{tmp_path / 'panel.db'}"
    (app_dir / ".env").write_text(f"SQLALCHEMY_DATABASE_URL={expected_url}\n", encoding="utf-8")

    runner = bin_dir / "rebecca-cli.py"
    runner.write_text("import config\nprint(config.SQLALCHEMY_DATABASE_URL)\n", encoding="utf-8")

    env = os.environ.copy()
    env.pop("REBECCA_ENV_FILE", None)
    env.pop("SQLALCHEMY_DATABASE_URL", None)
    existing_pythonpath = env.get("PYTHONPATH", "")
    env["PYTHONPATH"] = str(PROJECT_ROOT) if not existing_pythonpath else f"{PROJECT_ROOT}{os.pathsep}{existing_pythonpath}"

    result = subprocess.run(
        [sys.executable, str(runner)],
        cwd=tmp_path,
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )

    assert result.stdout.strip() == expected_url


def test_binary_runtime_info_supports_binary_mode_without_docker(tmp_path: Path, monkeypatch):
    app_dir = tmp_path / "rebecca"
    app_dir.mkdir()
    (app_dir / ".binary-release.json").write_text(
        json.dumps(
            {
                "install_mode": "binary",
                "image": "rebecca-server (binary)",
                "tag": "v1.2.3",
                "asset_url": "https://example.invalid/rebecca-linux-amd64.tar.gz",
            }
        ),
        encoding="utf-8",
    )

    monkeypatch.setenv("REBECCA_APP_DIR", str(app_dir))
    monkeypatch.setenv("REBECCA_SCRIPT_BIN", str(REBECCA_SCRIPT_PATH))
    monkeypatch.setenv("REBECCA_INSTALL_MODE", "binary")
    monkeypatch.setenv("REBECCA_BINARY_METADATA_FILE", str(app_dir / ".binary-release.json"))

    from app.utils.binary_control import get_binary_runtime_info

    panel_info = get_binary_runtime_info()
    assert panel_info["mode"] == "binary"
    assert panel_info["image"] == "rebecca-server (binary)"
    assert panel_info["tag"] == "v1.2.3"
    assert panel_info["channel"] == "latest"
    assert panel_info["binary"]["image"] == "rebecca-server (binary)"
    assert panel_info["binary"]["tag"] == "v1.2.3"


def test_build_rebecca_update_args_supports_release_and_dev_channels():
    from app.utils.binary_control import build_rebecca_update_args

    assert build_rebecca_update_args() == ["update"]
    assert build_rebecca_update_args(channel="latest") == ["update", "--version", "latest"]
    assert build_rebecca_update_args(channel="dev") == ["update", "--dev"]
    assert build_rebecca_update_args(version="v1.2.3") == ["update", "--version", "v1.2.3"]
    assert build_rebecca_update_args(version="latest") == ["update", "--version", "latest"]
    assert build_rebecca_update_args(version="dev-abcdef0") == ["update", "--version", "dev-abcdef0"]


def test_binary_update_status_reads_latest_dev_from_manifest(monkeypatch):
    from app.utils import update_check

    update_check._CACHE.clear()
    requested_urls: list[str] = []

    class FakeResponse:
        def __init__(self, payload: dict):
            self.payload = payload

        def raise_for_status(self) -> None:
            return None

        def json(self) -> dict:
            return self.payload

    def fake_get(url: str, **kwargs):
        requested_urls.append(url)
        if url == "https://api.github.com/repos/rebeccapanel/Rebecca/releases/latest":
            return FakeResponse({"tag_name": "v0.1.3", "html_url": "https://github.com/rebeccapanel/Rebecca/releases/tag/v0.1.3"})
        if url == "https://raw.githubusercontent.com/rebeccapanel/Rebecca/dev-build-manifest/dev-builds.json":
            return FakeResponse(
                {
                    "latest": "dev-abc1234",
                    "updated_at": "2026-06-03T08:00:00Z",
                    "builds": [
                        {
                            "tag": "dev-abc1234",
                            "sha": "abc1234567890",
                            "branch": "dev",
                            "run_id": "12345",
                            "created_at": "2026-06-03T07:59:00Z",
                            "assets": {
                                "linux-amd64": {
                                    "name": "rebecca-linux-amd64-dev-abc1234.tar.gz",
                                    "url": "https://github.com/rebeccapanel/Rebecca/releases/download/dev-builds/rebecca-linux-amd64-dev-abc1234.tar.gz",
                                }
                            },
                        }
                    ],
                }
            )
        raise AssertionError(f"Unexpected GitHub request: {url}")

    monkeypatch.setattr(update_check.requests, "get", fake_get)

    status = update_check.get_binary_update_status("rebeccapanel/Rebecca", "v0.1.3", channel="dev")

    assert status["target"] == "dev-abc1234"
    assert status["available"] is True
    assert status["latest_dev"]["tag"] == "dev-abc1234"
    assert status["latest_dev"]["html_url"] == "https://github.com/rebeccapanel/Rebecca/actions/runs/12345"
    assert (
        status["latest_dev"]["manifest_url"]
        == "https://raw.githubusercontent.com/rebeccapanel/Rebecca/dev-build-manifest/dev-builds.json"
    )
    assert status["latest_dev"]["assets"]["linux-amd64"]["url"].endswith("rebecca-linux-amd64-dev-abc1234.tar.gz")
    assert all("/actions/runs" not in url for url in requested_urls)


def test_runtime_info_defaults_to_docker_without_binary_marker(tmp_path: Path, monkeypatch):
    monkeypatch.setenv("REBECCA_APP_DIR", str(tmp_path / "rebecca"))
    monkeypatch.delenv("REBECCA_INSTALL_MODE", raising=False)
    monkeypatch.delenv("REBECCA_BINARY_METADATA_FILE", raising=False)

    from app.utils.binary_control import get_binary_runtime_info, is_binary_runtime

    panel_info = get_binary_runtime_info()
    assert panel_info["mode"] == "docker"
    assert is_binary_runtime() is False


def test_cli_help_skips_dashboard_runtime(tmp_path: Path):
    env = os.environ.copy()
    env["REBECCA_ENV_FILE"] = str(tmp_path / ".env")
    env["SQLALCHEMY_DATABASE_URL"] = f"sqlite:///{tmp_path / 'cli.db'}"
    existing_pythonpath = env.get("PYTHONPATH", "")
    env["PYTHONPATH"] = str(PROJECT_ROOT) if not existing_pythonpath else f"{PROJECT_ROOT}{os.pathsep}{existing_pythonpath}"

    result = subprocess.run(
        [sys.executable, str(REBECCA_CLI_PATH), "--help"],
        cwd=tmp_path,
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )

    assert "admin" in result.stdout
