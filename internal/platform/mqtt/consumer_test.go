package mqtt

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/memory"
	"iot-perform/internal/platform/repository"
)

func TestServiceAsyncConsumptionPipeline(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D", DeviceSecret: "secret"})

	service := NewServiceWithClient(nil, store.Repositories(), nil)
	service.workers = 2
	service.queueSize = 16
	service.batchSize = 1 // 每样本即刷，便于断言
	service.startConsumers()
	defer service.stopConsumers()

	telemetry := []byte(`{"ts":1722000000000,"values":{"temperature":41}}`)
	service.route("devices/pk/d1/telemetry", telemetry)
	service.route("devices/pk/d1/telemetry", telemetry)

	waitFor(t, 2*time.Second, func() bool {
		items, err := store.QueryTelemetry(ctx, repository.TelemetryQuery{DeviceID: "d1"})
		return err == nil && len(items) == 2
	})
}

func TestServiceRouteShardsByDevice(t *testing.T) {
	service := NewServiceWithClient(nil, repository.Repositories{}, nil)
	service.workers = 8
	service.queues = make([]chan ingestion, service.workers)
	for i := range service.queues {
		service.queues[i] = make(chan ingestion, 1)
	}
	if a, b := shardIndex("d-1", 8), shardIndex("d-1", 8); a != b {
		t.Fatalf("same device must map to the same shard: %d vs %d", a, b)
	}
	if a, b := shardIndex("d-1", 8), shardIndex("d-2", 8); a == b {
		t.Fatalf("distinct devices should not all land on shard %d", a)
	}
}

func TestServiceRouteDropsWhenQueueFull(t *testing.T) {
	metrics := &recordingMetrics{}
	service := NewServiceWithClient(nil, repository.Repositories{}, nil)
	service.metrics = metrics
	service.workers = 1
	service.queueSize = 1
	service.queues = []chan ingestion{make(chan ingestion, 1)}
	service.queues[0] <- ingestion{topic: "devices/pk/d1/telemetry", payload: []byte("{}")}
	service.route("devices/pk/d1/telemetry", []byte(`{}`))
	service.route("devices/pk/d1/telemetry", []byte(`{}`))
	if got := metrics.dropped(); got == 0 {
		t.Fatal("expected a dropped counter when the shard queue is full")
	}
}

func TestServiceProductCacheUsedByTelemetry(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D", DeviceSecret: "secret"})

	service := NewServiceWithClient(nil, store.Repositories(), nil)
	service.batchSize = 1
	service.startConsumers()
	defer service.stopConsumers()

	var queries atomic.Int32
	wrapped := wrapProductsQuery(service, &queries)
	service.products = wrapped

	telemetry := []byte(`{"ts":1722000000000,"values":{"temperature":41}}`)
	service.route("devices/pk/d1/telemetry", telemetry)
	service.route("devices/pk/d1/telemetry", telemetry)

	waitFor(t, 2*time.Second, func() bool {
		items, err := store.QueryTelemetry(ctx, repository.TelemetryQuery{DeviceID: "d1"})
		return err == nil && len(items) == 2
	})
	// 两条消息只查一次产品(缓存命中)
	if got := queries.Load(); got != 1 {
		t.Fatalf("expected 1 product lookup for 2 messages, got %d", got)
	}
}

// wrapProductsQuery wraps the service's product loader with a query counter to
// prove the cache collapses repeated lookups for the same product.
func wrapProductsQuery(service *Service, counter *atomic.Int32) *productCache {
	if service.repos.Products == nil {
		return nil
	}
	return newProductCache(func(ctx context.Context, key string) (domain.Product, error) {
		counter.Add(1)
		return service.repos.Products.GetProductByKey(ctx, key)
	}, time.Minute)
}
