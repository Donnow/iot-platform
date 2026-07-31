package mqtt

import (
	"context"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/memory"
	"iot-perform/internal/platform/repository"
)

func TestParseDeviceTopic(t *testing.T) {
	product, device, suffix, err := ParseDeviceTopic("devices/pk/d1/command/reply")
	if err != nil || product != "pk" || device != "d1" || suffix != "command/reply" {
		t.Fatalf("parsed topic = %q %q %q err=%v", product, device, suffix, err)
	}
	if _, _, _, err := ParseDeviceTopic("invalid/topic"); err == nil {
		t.Fatal("invalid topic should fail")
	}
}

func TestProcessTelemetryCommandReplyAndShadow(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D", DeviceSecret: "secret"})
	service := NewServiceWithClient(nil, store.Repositories(), nil)
	telemetry := []byte(`{"ts":1722000000000,"values":{"temperature":41}}`)
	if err := service.ProcessMessage(ctx, "devices/pk/d1/telemetry", telemetry); err != nil {
		t.Fatal(err)
	}
	items, err := store.QueryTelemetry(ctx, repository.TelemetryQuery{DeviceID: "d1"})
	if err != nil || len(items) != 1 {
		t.Fatalf("telemetry=%#v err=%v", items, err)
	}
	_, _ = store.CreateCommand(ctx, domain.Command{ID: "cmd-1", DeviceID: "d1", Method: "open"})
	if err := service.ProcessMessage(ctx, "devices/pk/d1/command/reply", []byte(`{"command_id":"cmd-1","code":0,"message":"ok"}`)); err != nil {
		t.Fatal(err)
	}
	command, err := store.GetCommand(ctx, "d1", "cmd-1")
	if err != nil || command.Status != domain.CommandStatusSuccess {
		t.Fatalf("command=%#v err=%v", command, err)
	}
	if err := service.ProcessMessage(ctx, "devices/pk/d1/shadow/reported", []byte(`{"reported":{"targetTemp":26}}`)); err != nil {
		t.Fatal(err)
	}
	shadow, err := store.GetShadow(ctx, "d1")
	if err != nil || shadow.Reported["targetTemp"] != float64(26) {
		t.Fatalf("shadow=%#v err=%v", shadow, err)
	}
}

func TestAuthenticateReturnsDeviceScopedACL(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D", DeviceSecret: "secret"})
	service := NewServiceWithClient(nil, store.Repositories(), nil)
	result, err := service.Authenticate(ctx, "d1", "secret")
	if err != nil || !result.Allow || len(result.ACL) != 7 {
		t.Fatalf("auth=%#v err=%v", result, err)
	}
	for _, rule := range result.ACL {
		if rule.Topic == "" || rule.Topic == "devices/+/+/telemetry" {
			t.Fatalf("unscoped ACL rule=%#v", rule)
		}
	}
	result, err = service.Authenticate(ctx, "d1", "bad")
	if err != nil || result.Allow {
		t.Fatalf("bad auth=%#v err=%v", result, err)
	}
}

func TestSetLifecycle(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D"})
	service := NewServiceWithClient(nil, store.Repositories(), nil)
	if err := service.SetLifecycle(ctx, "d1", true, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	device, err := store.GetDevice(ctx, "d1")
	if err != nil || device.Status != domain.DeviceStatusOnline {
		t.Fatalf("device=%#v err=%v", device, err)
	}
}
