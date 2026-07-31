package memory

import (
	"context"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func TestStoreProductDeviceAndAuthentication(t *testing.T) {
	store := New()
	ctx := context.Background()
	_, err := store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "Temperature", DeviceType: domain.DeviceTypeSensor})
	if err != nil {
		t.Fatal(err)
	}
	device, err := store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", DeviceSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if device.Status != domain.DeviceStatusInactive {
		t.Fatalf("status = %q", device.Status)
	}
	if _, err := store.AuthenticateDevice(ctx, "d1", "bad"); err == nil {
		t.Fatal("bad credentials should fail")
	}
	if _, err := store.AuthenticateDevice(ctx, "d1", "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestStoreTelemetrySnapshotAndShadowDelta(t *testing.T) {
	store := New()
	ctx := context.Background()
	first := time.Unix(100, 0).UTC()
	second := time.Unix(200, 0).UTC()
	if err := store.AppendTelemetry(ctx, domain.Telemetry{DeviceID: "d1", Timestamp: first, Values: map[string]any{"temperature": 20.0}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTelemetry(ctx, domain.Telemetry{DeviceID: "d1", Timestamp: second, Values: map[string]any{"temperature": 22.0, "humidity": 50.0}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.SnapshotTelemetry(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["temperature"].Values["temperature"] != 22.0 || len(snapshot) != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	shadow, err := store.UpsertDesired(ctx, "d1", map[string]any{"targetTemp": 26.0})
	if err != nil || shadow.Delta["targetTemp"] != 26.0 {
		t.Fatalf("desired shadow = %#v, err = %v", shadow, err)
	}
	shadow, err = store.UpsertReported(ctx, "d1", map[string]any{"targetTemp": 26.0})
	if err != nil || len(shadow.Delta) != 0 {
		t.Fatalf("reported shadow = %#v, err = %v", shadow, err)
	}
}

func TestStorePaginationAndCommandTimeout(t *testing.T) {
	store := New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk"})
	for i := 0; i < 3; i++ {
		_, err := store.CreateProduct(ctx, domain.Product{ProductKey: string(rune('a' + i)), Name: "P"})
		if err != nil {
			t.Fatal(err)
		}
	}
	products, page, err := store.ListProducts(ctx, repository.ProductFilter{Page: 2, PageSize: 2})
	if err != nil || len(products) != 2 || page.Total != 4 {
		t.Fatalf("products=%#v page=%#v err=%v", products, page, err)
	}
	created, err := store.CreateCommand(ctx, domain.Command{DeviceID: "d1", Method: "ping", CreatedAt: time.Now().Add(-31 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	command, err := store.GetCommand(ctx, "d1", created.ID)
	if err != nil || command.Status != domain.CommandStatusTimeout {
		t.Fatalf("command=%#v err=%v", command, err)
	}
}
