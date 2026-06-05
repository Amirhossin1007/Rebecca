package gateway

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestIsNativeNodeRoute(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		header string
		want   bool
	}{
		{name: "nodes list", method: http.MethodGet, path: "/api/nodes", want: true},
		{name: "nodes usage", method: http.MethodGet, path: "/api/nodes/usage", want: true},
		{name: "node get", method: http.MethodGet, path: "/api/node/12", want: true},
		{name: "node reconnect", method: http.MethodPost, path: "/api/node/12/reconnect", want: true},
		{name: "node restart", method: http.MethodPost, path: "/api/node/12/restart", want: true},
		{name: "node sync", method: http.MethodPost, path: "/api/node/12/sync", want: true},
		{name: "node logs", method: http.MethodGet, path: "/api/node/12/logs", want: true},
		{name: "node usage daily", method: http.MethodGet, path: "/api/node/12/usage/daily", want: true},
		{name: "node runtime update", method: http.MethodPost, path: "/api/node/12/xray/update", want: true},
		{name: "node geo update", method: http.MethodPost, path: "/api/node/12/geo/update", want: true},
		{name: "node service restart", method: http.MethodPost, path: "/api/node/12/service/restart", want: true},
		{name: "node service update", method: http.MethodPost, path: "/api/node/12/service/update", want: true},
		{name: "node websocket logs stays python", method: http.MethodGet, path: "/api/node/12/logs", header: "websocket", want: false},
		{name: "node create stays python", method: http.MethodPost, path: "/api/node", want: false},
		{name: "node update stays python", method: http.MethodPut, path: "/api/node/12", want: false},
		{name: "runtime route stays python", method: http.MethodGet, path: "/api/core", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Upgrade", tt.header)
			}
			if got := isNativeNodeRoute(req); got != tt.want {
				t.Fatalf("isNativeNodeRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNativeAdminRoute(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		header string
		want   bool
	}{
		{name: "current admin", method: http.MethodGet, path: "/api/admin", want: true},
		{name: "api admin token", method: http.MethodPost, path: "/api/admin/token", want: true},
		{name: "frontend admin token alias", method: http.MethodPost, path: "/admin/token", want: true},
		{name: "admin create", method: http.MethodPost, path: "/api/admin", want: true},
		{name: "admin list", method: http.MethodGet, path: "/api/admins", want: true},
		{name: "admin update", method: http.MethodPut, path: "/api/admin/seller", want: true},
		{name: "admin usage chart", method: http.MethodGet, path: "/api/admin/seller/usage/chart", want: true},
		{name: "myaccount get", method: http.MethodGet, path: "/api/myaccount", want: true},
		{name: "myaccount password", method: http.MethodPost, path: "/api/myaccount/change_password", want: true},
		{name: "myaccount api key delete", method: http.MethodDelete, path: "/api/myaccount/api-keys/7", want: true},
		{name: "admin websocket stays python", method: http.MethodGet, path: "/api/admin", header: "websocket", want: false},
		{name: "settings admins stays python", method: http.MethodPut, path: "/api/settings/subscriptions/admins/1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Upgrade", tt.header)
			}
			if got := isNativeAdminRoute(req); got != tt.want {
				t.Fatalf("isNativeAdminRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNativeUserRoute(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		header string
		want   bool
	}{
		{name: "users list", method: http.MethodGet, path: "/api/users", want: true},
		{name: "users list trailing slash", method: http.MethodGet, path: "/api/users/", want: true},
		{name: "users usage", method: http.MethodGet, path: "/api/users/usage", want: true},
		{name: "user detail", method: http.MethodGet, path: "/api/user/alice", want: true},
		{name: "user detail url encoded", method: http.MethodGet, path: "/api/user/alice%20vpn", want: true},
		{name: "user create", method: http.MethodPost, path: "/api/user", want: true},
		{name: "user create v2", method: http.MethodPost, path: "/api/v2/users", want: true},
		{name: "user update", method: http.MethodPut, path: "/api/user/alice", want: true},
		{name: "user update v2", method: http.MethodPut, path: "/api/v2/users/alice", want: true},
		{name: "user delete", method: http.MethodDelete, path: "/api/user/alice", want: true},
		{name: "user reset", method: http.MethodPost, path: "/api/user/alice/reset", want: true},
		{name: "user revoke sub", method: http.MethodPost, path: "/api/user/alice/revoke_sub", want: true},
		{name: "user active next", method: http.MethodPost, path: "/api/user/alice/active-next", want: true},
		{name: "user usage", method: http.MethodGet, path: "/api/user/alice/usage", want: true},
		{name: "users bulk action", method: http.MethodPost, path: "/api/users/actions", want: true},
		{name: "service scoped users bulk action", method: http.MethodPost, path: "/api/v2/services/7/users/actions", want: true},
		{name: "service scoped users bulk action bad id stays python", method: http.MethodPost, path: "/api/v2/services/nope/users/actions", want: false},
		{name: "service scoped users bulk action wrong method stays python", method: http.MethodGet, path: "/api/v2/services/7/users/actions", want: false},
		{name: "user websocket stays python", method: http.MethodGet, path: "/api/user/alice", header: "websocket", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Upgrade", tt.header)
			}
			if got := isNativeUserRoute(req); got != tt.want {
				t.Fatalf("isNativeUserRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNativeSubscriptionRoute(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		header   string
		prefixes []string
		want     bool
	}{
		{name: "sub token", method: http.MethodGet, path: "/sub/token", want: true},
		{name: "sub token info", method: http.MethodGet, path: "/sub/token/info", want: true},
		{name: "sub username key", method: http.MethodGet, path: "/sub/alice/key", want: true},
		{name: "subscribe query alias", method: http.MethodGet, path: "/api/v1/client/subscribe", want: true},
		{name: "subscribe path alias", method: http.MethodGet, path: "/api/v1/client/subscribe/token", want: true},
		{name: "custom prefix", method: http.MethodGet, path: "/my-sub/token", prefixes: []string{"/my-sub"}, want: true},
		{name: "post stays python", method: http.MethodPost, path: "/sub/token", want: false},
		{name: "websocket stays python", method: http.MethodGet, path: "/sub/token", header: "websocket", want: false},
		{name: "dashboard stays python", method: http.MethodGet, path: "/dashboard/login", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Upgrade", tt.header)
			}
			if got := isNativeSubscriptionRoute(req, tt.prefixes); got != tt.want {
				t.Fatalf("isNativeSubscriptionRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNativeNodeRouteDoesNotFallbackToPython(t *testing.T) {
	python := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("python fallback"))
	}))
	defer python.Close()

	pythonURL := strings.TrimPrefix(python.URL, "http://")
	host, portValue, err := net.SplitHostPort(pythonURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(Config{
		MasterAPIURL:     "http://127.0.0.1:1",
		NativeNodeRoutes: true,
		PythonHost:       host,
		PythonPort:       port,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "python fallback") {
		t.Fatalf("native node route fell back to python: %s", rec.Body.String())
	}
}

func TestNativeSubscriptionRouteDoesNotFallbackToPython(t *testing.T) {
	python := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("python fallback"))
	}))
	defer python.Close()

	pythonURL := strings.TrimPrefix(python.URL, "http://")
	host, portValue, err := net.SplitHostPort(pythonURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(Config{
		MasterAPIURL:             "http://127.0.0.1:1",
		NativeSubscriptionRoutes: true,
		PythonHost:               host,
		PythonPort:               port,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub/token", nil)
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "python fallback") {
		t.Fatalf("native subscription route fell back to python: %s", rec.Body.String())
	}
}

func TestNativeUserRouteDoesNotFallbackToPython(t *testing.T) {
	python := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("python fallback"))
	}))
	defer python.Close()

	pythonURL := strings.TrimPrefix(python.URL, "http://")
	host, portValue, err := net.SplitHostPort(pythonURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(Config{
		MasterAPIURL: "http://127.0.0.1:1",
		PythonHost:   host,
		PythonPort:   port,
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/users"},
		{method: http.MethodGet, path: "/api/users/usage"},
		{method: http.MethodGet, path: "/api/user/alice"},
		{method: http.MethodGet, path: "/api/user/alice/usage"},
		{method: http.MethodPost, path: "/api/user"},
		{method: http.MethodPost, path: "/api/v2/users"},
		{method: http.MethodPut, path: "/api/user/alice"},
		{method: http.MethodPut, path: "/api/v2/users/alice"},
		{method: http.MethodDelete, path: "/api/user/alice"},
		{method: http.MethodPost, path: "/api/user/alice/reset"},
		{method: http.MethodPost, path: "/api/user/alice/revoke_sub"},
		{method: http.MethodPost, path: "/api/user/alice/active-next"},
		{method: http.MethodPost, path: "/api/users/actions"},
		{method: http.MethodPost, path: "/api/v2/services/7/users/actions"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			server.server.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "python fallback") {
				t.Fatalf("native user route fell back to python: %s", rec.Body.String())
			}
		})
	}
}

func TestNativeAdminRouteDoesNotFallbackToPython(t *testing.T) {
	python := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("python fallback"))
	}))
	defer python.Close()

	pythonURL := strings.TrimPrefix(python.URL, "http://")
	host, portValue, err := net.SplitHostPort(pythonURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(Config{
		MasterAPIURL: "http://127.0.0.1:1",
		PythonHost:   host,
		PythonPort:   port,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admins", nil)
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "python fallback") {
		t.Fatalf("native admin route fell back to python: %s", rec.Body.String())
	}
}

func TestNativeAdminRouteProxiesToGoMasterAPI(t *testing.T) {
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected master request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"go-token","token_type":"bearer"}`))
	}))
	defer master.Close()

	python := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("python fallback"))
	}))
	defer python.Close()

	pythonURL := strings.TrimPrefix(python.URL, "http://")
	host, portValue, err := net.SplitHostPort(pythonURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(Config{
		MasterAPIURL: master.URL,
		PythonHost:   host,
		PythonPort:   port,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/token", strings.NewReader("username=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "python fallback") {
		t.Fatalf("native admin route fell back to python: %s", rec.Body.String())
	}
}
