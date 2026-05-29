from app.models.proxy import ProxyTypes, VLESSSettings
from app.subscription.share import generate_v2ray_links


def test_no_service_subscription_falls_back_to_enabled_hosts_when_inbounds_missing():
    inbounds_by_tag = {
        "VLESS TCP": {
            "tag": "VLESS TCP",
            "protocol": "vless",
            "port": 443,
            "network": "tcp",
            "tls": "none",
            "sni": [],
            "host": [],
            "path": "",
            "header_type": "none",
        }
    }
    host_map = {
        "VLESS TCP": [
            {
                "remark": "Test {USERNAME}",
                "address": ["example.com"],
                "port": 443,
                "path": None,
                "sni": [],
                "host": [],
                "alpn": "",
                "fingerprint": "",
                "tls": None,
                "allowinsecure": False,
                "mux_enable": False,
                "fragment_setting": None,
                "noise_setting": None,
                "random_user_agent": False,
                "use_sni_as_host": False,
                "sort": 0,
                "id": 1,
                "is_disabled": False,
                "inbound_tag": "VLESS TCP",
            }
        ]
    }

    links = generate_v2ray_links(
        proxies={ProxyTypes.VLESS: VLESSSettings(id="8fa6ab13-f503-f5e3-19cf-eb8601d5baca")},
        inbounds={ProxyTypes.VLESS: []},
        extra_data={
            "username": "fallback-user",
            "status": "active",
            "used_traffic": 0,
            "data_limit": None,
            "expire": None,
            "excluded_inbounds": {"vless": []},
        },
        reverse=False,
        inbounds_by_tag=inbounds_by_tag,
        host_map=host_map,
        force_refresh=False,
    )

    assert len(links) == 1
    assert links[0].startswith("vless://8fa6ab13-f503-f5e3-19cf-eb8601d5baca@example.com:443")
