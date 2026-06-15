package user

import (
	"strings"
	"testing"
)

func TestRenderClashLikeYAMLBuildsRealProxies(t *testing.T) {
	body := renderClashLikeYAML(
		"alice",
		[]string{
			"vless://7819215e-9bc0-7cdc-845b-16a174a7b6c6@example.com:443?security=tls&type=ws&path=%2Fws&host=edge.example.com&sni=edge.example.com&fp=chrome#edge",
			"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@example.net:8388#ss",
		},
		true,
	)
	for _, expected := range []string{
		`type: "vless"`,
		`server: "example.com"`,
		`uuid: "7819215e-9bc0-7cdc-845b-16a174a7b6c6"`,
		`ws-opts:`,
		`type: "ss"`,
		`cipher: "chacha20-ietf-poly1305"`,
		`password: "pass"`,
		`"♻️ Automatic"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in clash body:\n%s", expected, body)
		}
	}
	if strings.Contains(body, `url: "vless://`) || strings.Contains(body, `url: "ss://`) {
		t.Fatalf("clash proxies must not wrap share links as url-test URLs:\n%s", body)
	}
}

func TestSubscriptionPageTemplateIncludesLinks(t *testing.T) {
	var body strings.Builder
	err := subscriptionPageTemplate.Execute(&body, subscriptionHTMLData{
		Username:       "alice",
		Status:         "active",
		StatusClass:    "active",
		DataLimit:      "∞",
		DataUsed:       "1.00 MB",
		Expire:         "∞",
		Links:          []string{"vless://id@example.com:443#alice"},
		UsageURL:       "/sub/token/usage",
		HasActiveLinks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, expected := range []string{"Subscription Information", "User Information", "Links:", "vless://id@example.com:443#alice"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("expected %q in html:\n%s", expected, html)
		}
	}
}
