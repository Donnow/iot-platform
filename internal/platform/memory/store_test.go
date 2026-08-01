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

func TestStoreTelemetryAggregationAndOnlineProductCount(t *testing.T) {
	store := New()
	ctx := context.Background()
	base := time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC)
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D"})
	if err := store.SetDeviceStatus(ctx, "d1", domain.DeviceStatusOnline, &base); err != nil {
		t.Fatal(err)
	}
	for i, value := range []float64{20, 22, 24} {
		if err := store.AppendTelemetry(ctx, domain.Telemetry{DeviceID: "d1", ProductKey: "pk", Timestamp: base.Add(time.Duration(i)*time.Minute + 10*time.Second), Values: map[string]any{"temperature": value}}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.QueryTelemetry(ctx, repository.TelemetryQuery{DeviceID: "d1", Metric: "temperature", Interval: "1h"})
	if err != nil || len(items) != 1 || items[0].Values["temperature"] != 22.0 {
		t.Fatalf("aggregated telemetry=%#v err=%v", items, err)
	}
	products, _, err := store.ListProducts(ctx, repository.ProductFilter{Page: 1, PageSize: 10})
	if err != nil || len(products) != 1 || products[0].OnlineDeviceCount != 1 {
		t.Fatalf("products=%#v err=%v", products, err)
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

func TestStoreOTAProgressAndPendingTasks(t *testing.T) {
	store := New()
	ctx := context.Background()
	if _, err := store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"}); err != nil {
		t.Fatal(err)
	}
	for _, deviceID := range []string{"d1", "d2"} {
		if _, err := store.CreateDevice(ctx, domain.Device{DeviceID: deviceID, ProductKey: "pk", Name: deviceID}); err != nil {
			t.Fatal(err)
		}
	}
	firmware, err := store.CreateFirmware(ctx, domain.Firmware{ID: "fw-1", ProductKey: "pk", Version: "1.2.0", MD5: "0123456789abcdef0123456789abcdef", FileURL: "https://example.test/fw.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFirmware(ctx, domain.Firmware{ProductKey: "pk", Version: firmware.Version}); err != ErrConflict {
		t.Fatalf("duplicate firmware err=%v", err)
	}
	task, err := store.CreateOTATask(ctx, domain.OTATask{ProductKey: "pk", FirmwareID: firmware.ID, Version: firmware.Version, URL: firmware.FileURL, MD5: firmware.MD5, TargetDeviceIDs: []string{"d1", "d2"}})
	if err != nil {
		t.Fatal(err)
	}
	if task.Summary[domain.OTAStagePending] != 2 {
		t.Fatalf("initial summary=%#v", task.Summary)
	}
	if err := store.UpdateOTAProgress(ctx, task.ID, "d1", string(domain.OTAStageDownloading), 45, "downloading", time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateOTAProgress(ctx, task.ID, "d1", string(domain.OTAStageInstalling), 50, "installing", time.Unix(201, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateOTAProgress(ctx, task.ID, "d1", string(domain.OTAStageSuccess), 100, "ok", time.Unix(202, 0)); err != nil {
		t.Fatal(err)
	}
	task, err = store.GetOTATask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Summary[domain.OTAStageSuccess] != 1 || task.Summary[domain.OTAStagePending] != 1 {
		t.Fatalf("completed summary=%#v", task.Summary)
	}
	pending, err := store.ListPendingOTA(ctx, "d1")
	if err != nil || len(pending) != 0 {
		t.Fatalf("d1 pending=%#v err=%v", pending, err)
	}
	pending, err = store.ListPendingOTA(ctx, "d2")
	if err != nil || len(pending) != 1 || pending[0].ID != task.ID {
		t.Fatalf("d2 pending=%#v err=%v", pending, err)
	}
	if err := store.UpdateOTAProgress(ctx, task.ID, "d2", string(domain.OTAStageDownloading), 101, "", time.Time{}); err == nil {
		t.Fatal("out-of-range progress should fail")
	}
}
