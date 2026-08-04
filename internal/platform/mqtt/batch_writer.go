package mqtt

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

// batchWriter accumulates telemetry samples and flushes them as a single batch
// write, triggered either by batch size or a maximum wait. A flush that keeps
// failing is retried with backoff and then quarantined in an in-memory pending
// queue that a slower ticker keeps retrying, so a TDengine outage does not
// silently drop data and does not block the consumer workers.
type batchWriter struct {
	repo          repository.TelemetryRepository
	size          int
	maxWait       time.Duration
	retries       int
	retryBase     time.Duration
	retryInterval time.Duration
	pendingCap    int
	logger        *slog.Logger
	metrics       Metrics

	mu      sync.Mutex
	buf     []domain.Telemetry
	pending []domain.Telemetry
	notify  chan struct{}
}

type batchOptions struct {
	size          int
	maxWait       time.Duration
	retries       int
	retryBase     time.Duration
	retryInterval time.Duration
	pendingCap    int
}

func newBatchWriter(repo repository.TelemetryRepository, opts batchOptions, logger *slog.Logger, metrics Metrics) *batchWriter {
	if opts.size <= 0 {
		opts.size = 200
	}
	if opts.maxWait <= 0 {
		opts.maxWait = 200 * time.Millisecond
	}
	if opts.retries <= 0 {
		opts.retries = 3
	}
	if opts.retryBase <= 0 {
		opts.retryBase = 50 * time.Millisecond
	}
	if opts.retryInterval <= 0 {
		opts.retryInterval = 5 * time.Second
	}
	if opts.pendingCap <= 0 {
		opts.pendingCap = 1000
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &batchWriter{
		repo:          repo,
		size:          opts.size,
		maxWait:       opts.maxWait,
		retries:       opts.retries,
		retryBase:     opts.retryBase,
		retryInterval: opts.retryInterval,
		pendingCap:    opts.pendingCap,
		logger:        logger,
		metrics:       metrics,
		notify:        make(chan struct{}, 1),
	}
}

// run drives size/time-triggered flushes and the quarantined retry loop until
// ctx is cancelled, then flushes whatever is still buffered.
func (w *batchWriter) run(ctx context.Context) {
	ticker := time.NewTicker(w.maxWait)
	defer ticker.Stop()
	retryTicker := time.NewTicker(w.retryInterval)
	defer retryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.flushRemaining()
			return
		case <-ticker.C:
			w.flushDue()
		case <-retryTicker.C:
			w.retryPending()
		case <-w.notify:
			w.flushDue()
		}
	}
}

// Add appends one sample to the current batch. It never blocks on the flush
// itself; size-triggered flushes run on the batch writer's own goroutine.
func (w *batchWriter) Add(ctx context.Context, sample domain.Telemetry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	w.buf = append(w.buf, sample)
	trigger := len(w.buf) >= w.size
	w.mu.Unlock()
	if trigger {
		select {
		case w.notify <- struct{}{}:
		default:
		}
	}
	return nil
}

func (w *batchWriter) flushDue() {
	w.mu.Lock()
	if len(w.buf) == 0 {
		w.mu.Unlock()
		return
	}
	batch := w.buf
	w.buf = nil
	w.mu.Unlock()
	w.flushWithRetry(batch)
}

func (w *batchWriter) flushWithRetry(batch []domain.Telemetry) {
	backoff := w.retryBase
	var err error
	for attempt := 0; attempt <= w.retries; attempt++ {
		if err = w.repo.AppendTelemetryBatch(context.Background(), batch); err == nil {
			if w.metrics != nil {
				w.metrics.IncTelemetryBatchFlushed(len(batch))
			}
			return
		}
		if attempt < w.retries {
			if w.metrics != nil {
				w.metrics.IncTelemetryBatchRetries()
			}
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	if w.metrics != nil {
		w.metrics.IncTelemetryBatchFailures()
	}
	w.mu.Lock()
	if len(w.pending)+len(batch) > w.pendingCap {
		overflow := len(w.pending) + len(batch) - w.pendingCap
		w.pending = append(append([]domain.Telemetry(nil), w.pending[overflow:]...), batch...)
		if w.metrics != nil {
			w.metrics.IncMQTTDropped()
		}
	} else {
		w.pending = append(w.pending, batch...)
	}
	w.mu.Unlock()
	w.logger.Warn("telemetry batch flush failed; quarantined for retry", "rows", len(batch), "error", err)
}

func (w *batchWriter) retryPending() {
	w.mu.Lock()
	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}
	batch := w.pending
	w.pending = nil
	w.mu.Unlock()
	if err := w.repo.AppendTelemetryBatch(context.Background(), batch); err != nil {
		w.mu.Lock()
		w.pending = append(append([]domain.Telemetry(nil), batch...), w.pending...)
		w.mu.Unlock()
		w.logger.Warn("quarantined telemetry retry failed", "rows", len(batch), "error", err)
		return
	}
	if w.metrics != nil {
		w.metrics.IncTelemetryBatchFlushed(len(batch))
	}
	w.logger.Info("quarantined telemetry recovered", "rows", len(batch))
}

// flushRemaining writes whatever is still buffered or pending on shutdown,
// best-effort without retries.
func (w *batchWriter) flushRemaining() {
	w.mu.Lock()
	batch := w.buf
	pending := w.pending
	w.buf, w.pending = nil, nil
	w.mu.Unlock()
	if len(batch) > 0 {
		if err := w.repo.AppendTelemetryBatch(context.Background(), batch); err != nil {
			w.logger.Warn("final batch flush failed", "rows", len(batch), "error", err)
		}
	}
	if len(pending) > 0 {
		if err := w.repo.AppendTelemetryBatch(context.Background(), pending); err != nil {
			w.logger.Warn("final pending retry failed", "rows", len(pending), "error", err)
		}
	}
}
