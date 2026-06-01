package nodecontroller

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	nodev1 "github.com/rebeccapanel/rebecca/go/internal/proto/node/v1"
)

func (c Controller) List(ctx context.Context, req Request) (NodeListResult, error) {
	rows, defaultCert, err := c.repo.ListNodeItems(ctx, 0)
	if err != nil {
		return NodeListResult{}, err
	}
	for idx := range rows {
		enrichCertificateFields(&rows[idx], defaultCert)
		if rows[idx].Status == "disabled" || rows[idx].Status == "limited" {
			continue
		}
		metricsCtx, cancel := WithDefaultTimeout(ctx)
		runtime, err := c.Metrics(metricsCtx, Request{NodeID: rows[idx].ID})
		cancel()
		if err != nil {
			rows[idx].Status = "error"
			message := friendlyNodeError("metrics", rows[idx].ID, err).Error()
			rows[idx].Message = &message
			continue
		}
		applyRuntimeToNodeItem(&rows[idx], runtime)
	}
	return NodeListResult{Nodes: rows}, nil
}

func (c Controller) Get(ctx context.Context, req Request) (NodeListItem, error) {
	rows, defaultCert, err := c.repo.ListNodeItems(ctx, req.NodeID)
	if err != nil {
		return NodeListItem{}, err
	}
	if len(rows) == 0 {
		return NodeListItem{}, sql.ErrNoRows
	}
	item := rows[0]
	enrichCertificateFields(&item, defaultCert)
	if item.Status != "disabled" && item.Status != "limited" {
		metricsCtx, cancel := WithDefaultTimeout(ctx)
		runtime, err := c.Metrics(metricsCtx, Request{NodeID: item.ID})
		cancel()
		if err != nil {
			item.Status = "error"
			message := friendlyNodeError("metrics", item.ID, err).Error()
			item.Message = &message
			return item, nil
		}
		applyRuntimeToNodeItem(&item, runtime)
	}
	return item, nil
}

func (c Controller) Sync(ctx context.Context, req Request) (RuntimeResult, error) {
	node, err := c.repo.Node(ctx, req.NodeID)
	if err != nil {
		return RuntimeResult{}, err
	}
	configJSON := strings.TrimSpace(req.ConfigJSON)
	if configJSON == "" {
		configJSON, err = c.buildRuntimeConfig(ctx, node)
		if err != nil {
			return RuntimeResult{}, err
		}
	}
	client, _, err := c.dial(ctx, node.ID)
	if err != nil {
		_ = c.repo.SetError(ctx, node.ID, err.Error())
		return RuntimeResult{}, friendlyNodeError("sync", node.ID, err)
	}
	defer client.Close()
	res, err := client.Runtime().SyncConfig(ctx, &nodev1.RuntimeConfigRequest{
		OperationId: "sync-" + strconv.FormatInt(node.ID, 10),
		ConfigJson:  configJSON,
	})
	if err != nil {
		_ = c.repo.SetError(ctx, node.ID, err.Error())
		return RuntimeResult{}, friendlyNodeError("sync", node.ID, err)
	}
	return c.finishRuntime(ctx, node, res.GetRuntime(), res.GetMessage())
}

func applyRuntimeToNodeItem(item *NodeListItem, runtime RuntimeResult) {
	item.Status = runtime.Status
	if strings.TrimSpace(runtime.Message) != "" {
		item.Message = &runtime.Message
	}
	if strings.TrimSpace(runtime.XrayVersion) != "" {
		item.XrayVersion = &runtime.XrayVersion
	}
	if strings.TrimSpace(runtime.NodeServiceVersion) != "" {
		item.NodeServiceVersion = &runtime.NodeServiceVersion
	}
	if strings.TrimSpace(runtime.InstallMode) != "" {
		item.NodeInstallMode = &runtime.InstallMode
	}
	if strings.TrimSpace(runtime.UpdateChannel) != "" {
		item.NodeUpdateChannel = &runtime.UpdateChannel
	}
	item.CPU = runtime.CPU
	item.Memory = runtime.Memory
	item.Transfer = runtime.Transfer
}

func enrichCertificateFields(item *NodeListItem, defaultCert string) {
	cert := ""
	if item.NodeCertificate != nil {
		cert = strings.TrimSpace(*item.NodeCertificate)
	}
	defaultCert = strings.TrimSpace(defaultCert)
	if cert == "" || (defaultCert != "" && cert == defaultCert) {
		item.HasCustomCertificate = false
		item.UsesDefaultCertificate = true
		item.NodeCertificate = nil
		return
	}
	item.HasCustomCertificate = true
	item.UsesDefaultCertificate = false
}
