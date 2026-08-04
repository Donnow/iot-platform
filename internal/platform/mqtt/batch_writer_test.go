package mqtt

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

type fakeTelemetryRepo struct {
	mu        sync.Mutex
	rows      []domain.Telemetry
	remaining int // 剩余失败次数，>0 时 AppendTelemetryBatch 返回错误
	appends   atomic.Int32
}

func (f *fakeTelemetryRepo) AppendTelemetry(_ context.Context, sample domain.Telemetry) error {
	f.AppendTelemetryBatch(context.Background(), []domain.Telemetry{sample})
	return nil
}

func (f *fakeTelemetryRepo) AppendTelemetryBatch(_ context.Context, samples []domain.Telemetry) error {
	f.appends.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.remaining > 0 {
		f.remaining--
		return errors.New("tdengine unavailable")
	}
	f.rows = append(f.rows, samples...)
	return nil
}

func (f *fakeTelemetryRepo) QueryTelemetry(context.Context, repository.TelemetryQuery) ([]domain.Telemetry, error) {
	return nil, nil
}

func (f *fakeTelemetryRepo) SnapshotTelemetry(context.Context, string) (map[string]domain.Telemetry, error) {
	return nil, nil
}

type recordingMetrics struct {
	mu                         sync.Mutex
	mqttDropped                int
	telemetryBatchFlushed      int
	telemetryBatchRetries      int
	telemetryBatchFailures     int
	mqttMessages, mqttErrors   int
	ruleMatches, alarmsCreated int
}

func (m *recordingMetrics) IncMQTTMessages() { m.mu.Lock(); m.mqttMessages++; m.mu.Unlock() }
func (m *recordingMetrics) IncMQTTErrors()   { m.mu.Lock(); m.mqttErrors++; m.mu.Unlock() }
func (m *recordingMetrics) IncMQTTDropped()  { m.mu.Lock(); m.mqttDropped++; m.mu.Unlock() }
func (m *recordingMetrics) IncTelemetryBatchFlushed(rows int) {
	m.mu.Lock()
	m.telemetryBatchFlushed += rows
	m.mu.Unlock()
}
func (m *recordingMetrics) IncTelemetryBatchRetries() {
	m.mu.Lock()
	m.telemetryBatchRetries++
	m.mu.Unlock()
}
func (m *recordingMetrics) IncTelemetryBatchFailures() {
	m.mu.Lock()
	m.telemetryBatchFailures++
	m.mu.Unlock()
}
func (m *recordingMetrics) IncRuleMatches() { m.mu.Lock(); m.ruleMatches++; m.mu.Unlock() }
func (m *recordingMetrics) IncAlarmsCreated() {
	m.mu.Lock()
	m.alarmsCreated++
	m.mu.Unlock()
}

func (m *recordingMetrics) dropped() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mqttDropped
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func testBatchWriter(t *testing.T, opts batchOptions, repo *fakeTelemetryRepo) *batchWriter {
	t.Helper()
	if repo == nil {
		repo = &fakeTelemetryRepo{}
	}
	opts.retryInterval = 50 * time.Millisecond
	return newBatchWriter(repo, opts, nil, nil)
}

func runBatch(t *testing.T, writer *batchWriter) (context.CancelFunc, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		writer.run(ctx)
	}()
	return cancel, func() {
		cancel()
		<-done
	}
}

func TestBatchWriterFlushesOnSize(t *testing.T) {
	repo := &fakeTelemetryRepo{}
	writer := testBatchWriter(t, batchOptions{size: 2, maxWait: time.Second}, repo)
	cancel, wait := runBatch(t, writer)
	defer cancel()
	defer wait()

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_ = writer.Add(context.Background(), domain.Telemetry{DeviceID: "d1", Timestamp: base, Values: map[string]any{"t": 1.0}})
	_ = writer.Add(context.Background(), domain.Telemetry{DeviceID: "d1", Timestamp: base.Add(time.Second), Values: map[string]any{"t": 2.0}})

	waitFor(t, time.Second, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.rows) == 2
	})
	if got := repo.appends.Load(); got != 1 {
		t.Fatalf("expected a single batch flush, got %d", got)
	}
}

func TestBatchWriterFlushesOnMaxWait(t *testing.T) {
	repo := &fakeTelemetryRepo{}
	writer := testBatchWriter(t, batchOptions{size: 100, maxWait: 30 * time.Millisecond}, repo)
	cancel, wait := runBatch(t, writer)
	defer cancel()
	defer wait()

	_ = writer.Add(context.Background(), domain.Telemetry{DeviceID: "d1", Timestamp: time.Now(), Values: map[string]any{"t": 1.0}})
	waitFor(t, time.Second, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.rows) == 1
	})
}

func TestBatchWriterRetriesThenSucceeds(t *testing.T) {
	repo := &fakeTelemetryRepo{remaining: 2}
	writer := testBatchWriter(t, batchOptions{size: 1, maxWait: time.Second, retries: 3, retryBase: 5 * time.Millisecond}, repo)
	cancel, wait := runBatch(t, writer)
	defer cancel()
	defer wait()

	_ = writer.Add(context.Background(), domain.Telemetry{DeviceID: "d1", Timestamp: time.Now(), Values: map[string]any{"t": 1.0}})
	waitFor(t, 2*time.Second, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.rows) == 1
	})
}

func TestBatchWriterQuarantinesAfterRetriesExhausted(t *testing.T) {
	repo := &fakeTelemetryRepo{remaining: 100}
	metrics := &recordingMetrics{}
	writer := newBatchWriter(repo, batchOptions{size: 1, maxWait: time.Second, retries: 1, retryBase: 2 * time.Millisecond, retryInterval: 20 * time.Millisecond}, nil, metrics)
	cancel, wait := runBatch(t, writer)
	defer cancel()
	defer wait()

	_ = writer.Add(context.Background(), domain.Telemetry{DeviceID: "d1", Timestamp: time.Now(), Values: map[string]any{"t": 1.0}})

	// 重试耗尽后应隔离，指标累计
	waitFor(t, time.Second, func() bool {
		metrics.mu.Lock()
		defer metrics.mu.Unlock()
		return metrics.telemetryBatchFailures > 0
	})
	if repo.appends.Load() == 0 {
		t.Fatal("expected at least one flush attempt")
	}

	// 恢复后 pending 应被重放写回
	repo.mu.Lock()
	repo.remaining = 0
	repo.mu.Unlock()
	waitFor(t, 2*time.Second, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.rows) == 1
	})
}

func TestBatchWriterPendingCapDropsOldest(t *testing.T) {
	repo := &fakeTelemetryRepo{remaining: 100}
	metrics := &recordingMetrics{}
	writer := newBatchWriter(repo, batchOptions{size: 1, maxWait: time.Second, retries: 0, retryInterval: time.Hour, pendingCap: 1}, nil, metrics)
	cancel, wait := runBatch(t, writer)
	defer cancel()
	defer wait()

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_ = writer.Add(context.Background(), domain.Telemetry{DeviceID: "d1", Timestamp: base, Values: map[string]any{"t": 1.0}})
	time.Sleep(10 * time.Millisecond)
	_ = writer.Add(context.Background(), domain.Telemetry{DeviceID: "d2", Timestamp: base, Values: map[string]any{"t": 2.0}})

	waitFor(t, time.Second, func() bool {
		return metrics.dropped() > 0
	})
}
