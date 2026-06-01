package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	dashboardread "github.com/rebeccapanel/rebecca/go/internal/app/dashboard"
	"github.com/rebeccapanel/rebecca/go/internal/app/nodecontroller"
	"github.com/rebeccapanel/rebecca/go/internal/app/usage"
	userread "github.com/rebeccapanel/rebecca/go/internal/app/user"
	"github.com/rebeccapanel/rebecca/go/internal/platform/db"
)

type Request struct {
	Action      string          `json:"action"`
	DatabaseURL string          `json:"database_url"`
	Payload     json.RawMessage `json:"payload"`
}

type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

func Call(input []byte) []byte {
	var req Request
	if err := json.Unmarshal(input, &req); err != nil {
		return encodeError(err)
	}

	timeout := 10 * time.Second
	if isNodeAction(req.Action) {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pool, err := db.Open(req.DatabaseURL)
	if err != nil {
		return encodeError(err)
	}

	switch req.Action {
	case "usage.user":
		return handleUsageUser(ctx, pool, req.Payload)
	case "usage.user.timeseries":
		return handleUsageUserTimeseries(ctx, pool, req.Payload)
	case "usage.user.by_nodes":
		return handleUsageUserByNodes(ctx, pool, req.Payload)
	case "usage.admins":
		return handleUsageAdmins(ctx, pool, req.Payload)
	case "usage.admin.by_day":
		return handleUsageAdminByDay(ctx, pool, req.Payload)
	case "usage.admin.by_nodes":
		return handleUsageAdminByNodes(ctx, pool, req.Payload)
	case "usage.nodes":
		return handleUsageNodes(ctx, pool, req.Payload)
	case "usage.node.by_day":
		return handleUsageNodeByDay(ctx, pool, req.Payload)
	case "usage.service.timeseries":
		return handleUsageServiceTimeseries(ctx, pool, req.Payload)
	case "usage.service.admins":
		return handleUsageServiceAdmins(ctx, pool, req.Payload)
	case "usage.service.admin_timeseries":
		return handleUsageServiceAdminTimeseries(ctx, pool, req.Payload)
	case userread.ActionLinkPrerequisites:
		return handleUserLinkPrerequisites(ctx, pool, req.Payload)
	case userread.ActionSubscriptionLinks:
		return handleUserSubscriptionLinks(ctx, pool, req.Payload)
	case userread.ActionConfigLinks:
		return handleUserConfigLinks(ctx, pool, req.Payload)
	case userread.ActionUsersList:
		return handleUsersList(ctx, pool, req.Payload)
	case userread.ActionUserGet:
		return handleUserGet(ctx, pool, req.Payload)
	case dashboardread.ActionSystemSummary:
		return handleDashboardSystemSummary(ctx, pool, req.Payload)
	case nodecontroller.ActionList:
		return handleNodeList(ctx, pool, req.Payload)
	case nodecontroller.ActionGet:
		return handleNodeGet(ctx, pool, req.Payload)
	case nodecontroller.ActionSync:
		return handleNodeSync(ctx, pool, req.Payload)
	case nodecontroller.ActionUpdateRuntime:
		return handleNodeUpdateRuntime(ctx, pool, req.Payload)
	case nodecontroller.ActionUpdateGeo:
		return handleNodeUpdateGeo(ctx, pool, req.Payload)
	case nodecontroller.ActionRestartService:
		return handleNodeRestartService(ctx, pool, req.Payload)
	case nodecontroller.ActionUpdateService:
		return handleNodeUpdateService(ctx, pool, req.Payload)
	case nodecontroller.ActionConnect:
		return handleNodeConnect(ctx, pool, req.Payload)
	case nodecontroller.ActionReconnect:
		return handleNodeReconnect(ctx, pool, req.Payload)
	case nodecontroller.ActionRestart:
		return handleNodeRestart(ctx, pool, req.Payload)
	case nodecontroller.ActionHealth:
		return handleNodeHealth(ctx, pool, req.Payload)
	case nodecontroller.ActionMetrics:
		return handleNodeMetrics(ctx, pool, req.Payload)
	case nodecontroller.ActionLogs:
		return handleNodeLogs(ctx, pool, req.Payload)
	case nodecontroller.ActionProcessOperations:
		return handleNodeOperationsProcess(ctx, pool, req.Payload)
	case nodecontroller.ActionCollectUsage:
		return handleUsageCollect(ctx, pool, req.Payload)
	default:
		return encodeError(fmt.Errorf("unknown action: %s", req.Action))
	}
}

func isNodeAction(action string) bool {
	switch action {
	case nodecontroller.ActionList,
		nodecontroller.ActionGet,
		nodecontroller.ActionSync,
		nodecontroller.ActionUpdateRuntime,
		nodecontroller.ActionUpdateGeo,
		nodecontroller.ActionRestartService,
		nodecontroller.ActionUpdateService,
		nodecontroller.ActionConnect,
		nodecontroller.ActionReconnect,
		nodecontroller.ActionRestart,
		nodecontroller.ActionHealth,
		nodecontroller.ActionMetrics,
		nodecontroller.ActionLogs,
		nodecontroller.ActionProcessOperations,
		nodecontroller.ActionCollectUsage:
		return true
	default:
		return false
	}
}

func handleNodeList(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).List(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result.Nodes)
}

func handleNodeGet(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Get(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeSync(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Sync(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeUpdateRuntime(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).UpdateRuntime(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeUpdateGeo(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).UpdateGeo(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeRestartService(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).RestartService(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeUpdateService(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).UpdateService(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleUsageUser(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.UserUsage(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageUserTimeseries(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.UserUsageTimeseries(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageUserByNodes(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.UserUsageByNodes(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageAdmins(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.AdminsUsage(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageAdminByDay(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.AdminUsageByDay(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageAdminByNodes(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.AdminUsageByNodes(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageNodes(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.NodesUsage(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageNodeByDay(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.NodeUsageByDay(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageServiceTimeseries(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.ServiceUsageTimeseries(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageServiceAdmins(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.ServiceAdminUsage(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUsageServiceAdminTimeseries(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req usage.UsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := usage.NewService(usage.NewRepository(pool.DB, pool.Dialect))
	rows, err := service.ServiceAdminUsageTimeseries(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(rows)
}

func handleUserLinkPrerequisites(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req userread.LinkPrerequisitesRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := userread.NewService(userread.NewRepository(pool.DB, pool.Dialect))
	result, err := service.LinkPrerequisites(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleUserSubscriptionLinks(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req userread.SubscriptionLinkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := userread.NewService(userread.NewRepository(pool.DB, pool.Dialect))
	result, err := service.SubscriptionLinks(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleUserConfigLinks(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req userread.ConfigLinksRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := userread.NewService(userread.NewRepository(pool.DB, pool.Dialect))
	result, err := service.ConfigLinks(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleUsersList(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req userread.UsersListRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := userread.NewService(userread.NewRepository(pool.DB, pool.Dialect))
	result, err := service.UsersList(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleUserGet(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req userread.UserGetRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	service := userread.NewService(userread.NewRepository(pool.DB, pool.Dialect))
	result, err := service.UserGet(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleDashboardSystemSummary(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req dashboardread.SystemSummaryRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	result, err := dashboardread.NewRepository(pool.DB, pool.Dialect).SystemSummary(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeConnect(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := nodecontroller.WithDefaultTimeout(ctx)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Connect(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeReconnect(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := nodecontroller.WithDefaultTimeout(ctx)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Reconnect(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeRestart(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Restart(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeHealth(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := nodecontroller.WithDefaultTimeout(ctx)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Health(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeMetrics(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := nodecontroller.WithDefaultTimeout(ctx)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Metrics(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeLogs(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).Logs(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleNodeOperationsProcess(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.ProcessOperationsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).ProcessQueue(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func handleUsageCollect(ctx context.Context, pool db.Pool, payload json.RawMessage) []byte {
	var req nodecontroller.CollectUsageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return encodeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	result, err := nodecontroller.NewController(nodecontroller.NewRepository(pool.DB, pool.Dialect)).CollectUsage(ctx, req)
	if err != nil {
		return encodeError(err)
	}
	return encodeData(result)
}

func encodeData(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return encodeError(err)
	}
	response, err := json.Marshal(Response{OK: true, Data: data})
	if err != nil {
		return []byte(`{"ok":false,"error":"failed to encode response"}`)
	}
	return response
}

func encodeError(err error) []byte {
	response, marshalErr := json.Marshal(Response{OK: false, Error: err.Error()})
	if marshalErr != nil {
		return []byte(`{"ok":false,"error":"failed to encode error"}`)
	}
	return response
}
