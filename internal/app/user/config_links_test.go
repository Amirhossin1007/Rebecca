package user

import (
	"strings"
	"testing"
)

func TestBuildConfigLinksReplacesServerIPPlaceholder(t *testing.T) {
	serviceID := int64(1)
	links, err := BuildConfigLinks(
		ConfigLinkUser{
			ID:            7,
			Username:      "alice",
			Status:        "active",
			ServiceID:     &serviceID,
			CredentialKey: "05bfddf81eb418fa1edbce7cd286eee1",
			ServerIP:      "116.203.156.169",
			ServiceHostOrders: map[int64]int64{
				1: 0,
			},
		},
		map[string]ResolvedInbound{
			"Shadowsocks TCP": {
				"tag":      "Shadowsocks TCP",
				"protocol": "shadowsocks",
				"port":     int64(1080),
				"network":  "tcp",
			},
		},
		[]string{"Shadowsocks TCP"},
		[]Host{{
			ID:         1,
			InboundTag: "Shadowsocks TCP",
			Remark:     "Rebecca ({username})",
			Address:    "{SERVER_IP}",
			Security:   "inbound_default",
			ServiceIDs: []int64{1},
		}},
		map[string][]byte{},
		false,
	)
	if err != nil {
		t.Fatalf("BuildConfigLinks error: %v", err)
	}
	if len(links.Links) != 1 {
		t.Fatalf("expected one link, got %#v", links.Links)
	}
	if strings.Contains(links.Links[0], "{SERVER_IP}") || !strings.Contains(links.Links[0], "@116.203.156.169:1080") {
		t.Fatalf("server IP placeholder was not replaced: %s", links.Links[0])
	}
}
