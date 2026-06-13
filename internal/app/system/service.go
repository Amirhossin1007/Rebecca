package system

import (
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	dashboardapp "github.com/rebeccapanel/rebecca/internal/app/dashboard"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gonet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

const historyMaxEntries = 6000

type MetricsProvider interface {
	Snapshot(ctx context.Context) (MetricsSnapshot, error)
}

type Service struct {
	db        *sql.DB
	dashboard dashboardapp.Repository
	metrics   MetricsProvider
	version   string

	mu                 sync.Mutex
	cpuHistory         []HistoryEntry
	memoryHistory      []HistoryEntry
	networkHistory     []NetworkHistoryEntry
	panelCPUHistory    []HistoryEntry
	panelMemoryHistory []HistoryEntry
}

func NewService(db *sql.DB, dialect string, version string) *Service {
	return NewServiceWithProvider(db, dialect, version, NewGopsutilMetricsProvider())
}

func NewServiceWithProvider(db *sql.DB, dialect string, version string, provider MetricsProvider) *Service {
	if version == "" {
		version = DefaultVersion
	}
	if provider == nil {
		provider = NewGopsutilMetricsProvider()
	}
	return &Service{
		db:        db,
		dashboard: dashboardapp.NewRepository(db, dialect),
		metrics:   provider,
		version:   version,
	}
}

func (s *Service) Stats(ctx context.Context, admin dashboardapp.AdminContext) (SystemStats, error) {
	snapshot, err := s.metrics.Snapshot(ctx)
	if err != nil {
		return SystemStats{}, err
	}
	summary, err := s.dashboard.SystemSummary(ctx, dashboardapp.SystemSummaryRequest{Admin: admin})
	if err != nil {
		return SystemStats{}, err
	}
	xrayRunning, xrayVersion, err := s.connectedNodeRuntime(ctx)
	if err != nil {
		return SystemStats{}, err
	}
	history := s.appendHistory(snapshot)

	// TODO(go-telegram): populate last_telegram_error from the Go Telegram reporter once
	// Telegram reporting is restored natively.
	return SystemStats{
		Version:               s.version,
		CPUCores:              snapshot.CPUCores,
		CPUUsage:              snapshot.CPUUsage,
		TotalUser:             summary.TotalUser,
		OnlineUsers:           summary.OnlineUsers,
		UsersActive:           summary.UsersActive,
		UsersOnHold:           summary.UsersOnHold,
		UsersDisabled:         summary.UsersDisabled,
		UsersExpired:          summary.UsersExpired,
		UsersLimited:          summary.UsersLimited,
		IncomingBandwidth:     summary.IncomingBandwidth,
		OutgoingBandwidth:     summary.OutgoingBandwidth,
		PanelTotalBandwidth:   summary.PanelTotalBandwidth,
		IncomingBandwidthRate: snapshot.IncomingBandwidthSpeed,
		OutgoingBandwidthRate: snapshot.OutgoingBandwidthSpeed,
		Memory:                snapshot.Memory,
		Swap:                  snapshot.Swap,
		Disk:                  snapshot.Disk,
		LoadAvg:               snapshot.LoadAvg,
		UptimeSeconds:         snapshot.UptimeSeconds,
		PanelUptimeSeconds:    snapshot.PanelUptimeSeconds,
		XrayUptimeSeconds:     0,
		XrayRunning:           xrayRunning,
		XrayVersion:           xrayVersion,
		AppMemory:             snapshot.AppMemory,
		AppThreads:            snapshot.AppThreads,
		PanelCPUPercent:       snapshot.PanelCPUPercent,
		PanelMemoryPercent:    snapshot.PanelMemoryPercent,
		CPUHistory:            history.cpu,
		MemoryHistory:         history.memory,
		NetworkHistory:        history.network,
		PanelCPUHistory:       history.panelCPU,
		PanelMemoryHistory:    history.panelMemory,
		PersonalUsage:         summary.PersonalUsage,
		AdminOverview:         summary.AdminOverview,
		LastXrayError:         nil,
		LastTelegramError:     nil,
	}, nil
}

type historySnapshot struct {
	cpu         []HistoryEntry
	memory      []HistoryEntry
	network     []NetworkHistoryEntry
	panelCPU    []HistoryEntry
	panelMemory []HistoryEntry
}

func (s *Service) appendHistory(snapshot MetricsSnapshot) historySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	timestamp := snapshot.Timestamp
	if timestamp <= 0 {
		timestamp = time.Now().Unix()
	}
	s.cpuHistory = appendBounded(s.cpuHistory, HistoryEntry{Timestamp: timestamp, Value: snapshot.CPUUsage})
	s.memoryHistory = appendBounded(s.memoryHistory, HistoryEntry{Timestamp: timestamp, Value: snapshot.Memory.Percent})
	s.networkHistory = appendBounded(
		s.networkHistory,
		NetworkHistoryEntry{
			Timestamp: timestamp,
			Incoming:  snapshot.IncomingBandwidthSpeed,
			Outgoing:  snapshot.OutgoingBandwidthSpeed,
		},
	)
	s.panelCPUHistory = appendBounded(
		s.panelCPUHistory,
		HistoryEntry{Timestamp: timestamp, Value: snapshot.PanelCPUPercent},
	)
	s.panelMemoryHistory = appendBounded(
		s.panelMemoryHistory,
		HistoryEntry{Timestamp: timestamp, Value: snapshot.PanelMemoryPercent},
	)

	return historySnapshot{
		cpu:         append([]HistoryEntry(nil), s.cpuHistory...),
		memory:      append([]HistoryEntry(nil), s.memoryHistory...),
		network:     append([]NetworkHistoryEntry(nil), s.networkHistory...),
		panelCPU:    append([]HistoryEntry(nil), s.panelCPUHistory...),
		panelMemory: append([]HistoryEntry(nil), s.panelMemoryHistory...),
	}
}

func appendBounded[T any](items []T, item T) []T {
	items = append(items, item)
	if len(items) <= historyMaxEntries {
		return items
	}
	copy(items, items[len(items)-historyMaxEntries:])
	return items[:historyMaxEntries]
}

func (s *Service) connectedNodeRuntime(ctx context.Context) (bool, *string, error) {
	var version sql.NullString
	err := s.db.QueryRowContext(
		ctx,
		`SELECT xray_version FROM nodes WHERE LOWER(COALESCE(status, '')) = 'connected' ORDER BY id LIMIT 1`,
	).Scan(&version)
	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if version.Valid && version.String != "" {
		value := version.String
		return true, &value, nil
	}
	return true, nil, nil
}

type GopsutilMetricsProvider struct {
	mu        sync.Mutex
	process   *process.Process
	lastNet   *gonet.IOCountersStat
	lastNetAt time.Time
}

func NewGopsutilMetricsProvider() *GopsutilMetricsProvider {
	proc, _ := process.NewProcess(int32(os.Getpid()))
	if proc != nil {
		_, _ = proc.CPUPercent()
	}
	return &GopsutilMetricsProvider{process: proc}
}

func (p *GopsutilMetricsProvider) Snapshot(ctx context.Context) (MetricsSnapshot, error) {
	now := time.Now()
	result := MetricsSnapshot{Timestamp: now.Unix()}

	if cores, err := cpu.CountsWithContext(ctx, true); err == nil {
		result.CPUCores = cores
	}
	if percents, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(percents) > 0 {
		result.CPUUsage = finitePercent(percents[0])
	}
	if memory, err := mem.VirtualMemoryWithContext(ctx); err == nil && memory != nil {
		result.Memory = UsageStats{
			Current: int64(memory.Used),
			Total:   int64(memory.Total),
			Percent: finitePercent(memory.UsedPercent),
		}
	}
	if swap, err := mem.SwapMemoryWithContext(ctx); err == nil && swap != nil {
		result.Swap = UsageStats{
			Current: int64(swap.Used),
			Total:   int64(swap.Total),
			Percent: finitePercent(swap.UsedPercent),
		}
	}
	if usage, err := disk.UsageWithContext(ctx, diskRoot()); err == nil && usage != nil {
		result.Disk = UsageStats{
			Current: int64(usage.Used),
			Total:   int64(usage.Total),
			Percent: finitePercent(usage.UsedPercent),
		}
	}
	if avg, err := load.AvgWithContext(ctx); err == nil && avg != nil {
		result.LoadAvg = []float64{avg.Load1, avg.Load5, avg.Load15}
	} else {
		result.LoadAvg = []float64{}
	}
	if uptime, err := host.UptimeWithContext(ctx); err == nil {
		result.UptimeSeconds = int64(uptime)
	}
	p.populateProcess(ctx, now, &result)
	result.IncomingBandwidthSpeed, result.OutgoingBandwidthSpeed = p.realtimeBandwidth(ctx, now)
	return result, nil
}

func (p *GopsutilMetricsProvider) populateProcess(ctx context.Context, now time.Time, result *MetricsSnapshot) {
	if p.process == nil {
		return
	}
	if percent, err := p.process.CPUPercentWithContext(ctx); err == nil {
		result.PanelCPUPercent = finitePercent(percent)
	}
	if percent, err := p.process.MemoryPercentWithContext(ctx); err == nil {
		result.PanelMemoryPercent = finitePercent(float64(percent))
	}
	if createTime, err := p.process.CreateTimeWithContext(ctx); err == nil && createTime > 0 {
		result.PanelUptimeSeconds = maxInt64(0, now.Unix()-createTime/1000)
	}
	if info, err := p.process.MemoryInfoWithContext(ctx); err == nil && info != nil {
		result.AppMemory = int64(info.RSS)
	}
	if threads, err := p.process.NumThreadsWithContext(ctx); err == nil {
		result.AppThreads = int64(threads)
	}
}

func (p *GopsutilMetricsProvider) realtimeBandwidth(ctx context.Context, now time.Time) (int64, int64) {
	counters, err := gonet.IOCountersWithContext(ctx, false)
	if err != nil || len(counters) == 0 {
		return 0, 0
	}
	current := counters[0]
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastNet == nil || p.lastNetAt.IsZero() {
		p.lastNet = &current
		p.lastNetAt = now
		return 0, 0
	}
	elapsed := now.Sub(p.lastNetAt).Seconds()
	if elapsed <= 0 {
		p.lastNet = &current
		p.lastNetAt = now
		return 0, 0
	}
	incoming := int64(math.Round(float64(current.BytesRecv-p.lastNet.BytesRecv) / elapsed))
	outgoing := int64(math.Round(float64(current.BytesSent-p.lastNet.BytesSent) / elapsed))
	p.lastNet = &current
	p.lastNetAt = now
	if incoming < 0 {
		incoming = 0
	}
	if outgoing < 0 {
		outgoing = 0
	}
	return incoming, outgoing
}

func diskRoot() string {
	if runtime.GOOS != "windows" {
		return string(os.PathSeparator)
	}
	wd, err := os.Getwd()
	if err != nil {
		return `C:\`
	}
	volume := filepath.VolumeName(wd)
	if volume == "" {
		return `C:\`
	}
	return volume + `\`
}

func finitePercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
