package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/memory"
	"iot-perform/internal/platform/repository"
)

type recordingPublisher struct {
	messages []recordedMessage
}

type recordedMessage struct {
	topic   string
	payload []byte
}

func (p *recordingPublisher) Publish(_ context.Context, topic string, _ byte, _ bool, payload []byte) error {
	p.messages = append(p.messages, recordedMessage{topic: topic, payload: append([]byte(nil), payload...)})
	return nil
}

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
		if rule.Permission != "allow" || rule.Topic == "" || rule.Topic == "devices/+/+/telemetry" {
			t.Fatalf("unscoped ACL rule=%#v", rule)
		}
	}
	result, err = service.Authenticate(ctx, "d1", "bad")
	if err != nil || result.Allow {
		t.Fatalf("bad auth=%#v err=%v", result, err)
	}
}

func TestPlatformServiceAuthenticationAndACL(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D", DeviceSecret: "secret"})
	service, err := NewService(Config{
		BrokerURL: "tcp://broker:1883", ClientID: "iot-platform", Username: "iot-platform", Password: "platform-secret",
	}, store.Repositories(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Authenticate(ctx, "iot-platform", "platform-secret")
	if err != nil || !result.Allow {
		t.Fatalf("platform auth=%#v err=%v", result, err)
	}
	result, err = service.Authenticate(ctx, "iot-platform", "bad")
	if err != nil || result.Allow {
		t.Fatalf("bad platform auth=%#v err=%v", result, err)
	}
	allowed := []struct {
		topic  string
		action string
	}{
		{telemetrySubscription, "subscribe"},
		{"devices/+/+/telemetry", "subscribe"},
		{commandReplySubscription, "subscribe"},
		{"devices/pk/d1/command", "publish"},
		{"devices/pk/d1/shadow/desired", "publish"},
		{"devices/pk/d1/status", "publish"},
	}
	for _, test := range allowed {
		ok, err := service.Authorize(ctx, "iot-platform", test.topic, test.action)
		if err != nil || !ok {
			t.Fatalf("platform ACL topic=%q action=%q ok=%v err=%v", test.topic, test.action, ok, err)
		}
	}
	for _, test := range []struct {
		topic  string
		action string
	}{
		{"devices/pk/d1/telemetry", "subscribe"},
		{"devices/pk/d1/telemetry", "publish"},
		{"devices/pk/d1/event", "publish"},
	} {
		ok, err := service.Authorize(ctx, "iot-platform", test.topic, test.action)
		if err != nil || ok {
			t.Fatalf("unexpected platform ACL topic=%q action=%q ok=%v err=%v", test.topic, test.action, ok, err)
		}
	}
	ok, err := service.Authorize(ctx, "d1", "devices/pk/d1/telemetry", "publish")
	if err != nil || !ok {
		t.Fatalf("device ACL ok=%v err=%v", ok, err)
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

func TestSetLifecyclePublishesShadowAndPendingOTA(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D"})
	_, _ = store.CreateFirmware(ctx, domain.Firmware{ID: "fw-1", ProductKey: "pk", Version: "1.0.0", MD5: "0123456789abcdef0123456789abcdef", FileURL: "https://example.test/fw.bin"})
	_, err := store.CreateOTATask(ctx, domain.OTATask{ProductKey: "pk", FirmwareID: "fw-1", Version: "1.0.0", URL: "https://example.test/fw.bin", MD5: "0123456789abcdef0123456789abcdef", TargetDeviceIDs: []string{"d1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertDesired(ctx, "d1", map[string]any{"targetTemp": 26}); err != nil {
		t.Fatal(err)
	}
	publisher := &recordingPublisher{}
	service := NewServiceWithClient(nil, store.Repositories(), nil)
	service.publisher = publisher
	if err := service.SetLifecycle(ctx, "d1", true, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if len(publisher.messages) != 3 {
		t.Fatalf("published=%#v", publisher.messages)
	}
	if publisher.messages[0].topic != "devices/pk/d1/status" || publisher.messages[1].topic != "devices/pk/d1/shadow/desired" || publisher.messages[2].topic != "devices/pk/d1/ota" {
		t.Fatalf("topics=%#v", publisher.messages)
	}
	var ota map[string]any
	if err := json.Unmarshal(publisher.messages[2].payload, &ota); err != nil || ota["version"] != "1.0.0" {
		t.Fatalf("ota payload=%#v err=%v", ota, err)
	}
}

func TestProcessOTAProgress(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D"})
	_, _ = store.CreateFirmware(ctx, domain.Firmware{ID: "fw-1", ProductKey: "pk", Version: "1.0.0", MD5: "0123456789abcdef0123456789abcdef", FileURL: "https://example.test/fw.bin"})
	task, err := store.CreateOTATask(ctx, domain.OTATask{ProductKey: "pk", FirmwareID: "fw-1", Version: "1.0.0", URL: "https://example.test/fw.bin", MD5: "0123456789abcdef0123456789abcdef", TargetDeviceIDs: []string{"d1"}})
	if err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithClient(nil, store.Repositories(), nil)
	payload := []byte(`{"ts":1722000000000,"event_type":"ota_progress","data":{"version":"1.0.0","stage":"success","progress":100,"message":"ok"}}`)
	if err := service.ProcessMessage(ctx, "devices/pk/d1/event", payload); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetOTATask(ctx, task.ID)
	if err != nil || updated.Summary[domain.OTAStageSuccess] != 1 {
		t.Fatalf("task=%#v err=%v", updated, err)
	}
}

func TestProcessWillMessageUpdatesLifecycle(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D"})
	if err := store.SetDeviceStatus(ctx, "d1", domain.DeviceStatusOnline, nil); err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithClient(nil, store.Repositories(), nil)
	now := time.Now().UTC()
	if err := service.ProcessMessage(ctx, "devices/pk/d1/event", []byte(fmt.Sprintf(`{"status":"offline","ts":%d}`, now.UnixMilli()))); err != nil {
		t.Fatal(err)
	}
	device, err := store.GetDevice(ctx, "d1")
	if err != nil || device.Status != domain.DeviceStatusOffline {
		t.Fatalf("device=%#v err=%v", device, err)
	}
	if err := service.ProcessMessage(ctx, "devices/pk/d1/event", []byte(`{"status":"online"}`)); err != nil {
		t.Fatal(err)
	}
	device, err = store.GetDevice(ctx, "d1")
	if err != nil || device.Status != domain.DeviceStatusOnline {
		t.Fatalf("device after online=%#v err=%v", device, err)
	}
}

func TestProcessStaleWillIsIgnored(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D"})
	service := NewServiceWithClient(nil, store.Repositories(), nil)
	now := time.Now().UTC()
	if err := service.SetLifecycle(ctx, "d1", true, now); err != nil {
		t.Fatal(err)
	}
	oldWill := now.Add(-30 * time.Second)
	if err := service.ProcessMessage(ctx, "devices/pk/d1/event", []byte(fmt.Sprintf(`{"status":"offline","ts":%d}`, oldWill.UnixMilli()))); err != nil {
		t.Fatal(err)
	}
	device, err := store.GetDevice(ctx, "d1")
	if err != nil || device.Status != domain.DeviceStatusOnline {
		t.Fatalf("stale will flipped device=%#v err=%v", device, err)
	}
}

func TestTelemetryValidationAndRuleDuration(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	min, max := 0.0, 100.0
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P", Properties: []domain.Property{{
		Name: "temperature", DataType: domain.PropertyTypeFloat, MinValue: &min, MaxValue: &max,
	}}})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D"})
	_, _ = store.CreateRule(ctx, domain.Rule{ID: "r1", ProductKey: "pk", Name: "hot", PropertyName: "temperature", Operator: ">", Threshold: 40, DurationSeconds: 10, Enabled: true})
	service := NewServiceWithClient(nil, store.Repositories(), nil)

	for _, ts := range []int64{1000000, 1005000} {
		if err := service.ProcessMessage(ctx, "devices/pk/d1/telemetry", []byte(fmt.Sprintf(`{"ts":%d,"values":{"temperature":41}}`, ts))); err != nil {
			t.Fatal(err)
		}
	}
	alarms, _, err := store.ListAlarms(ctx, repository.AlarmFilter{DeviceID: "d1"})
	if err != nil || len(alarms) != 0 {
		t.Fatalf("alarms before duration=%#v err=%v", alarms, err)
	}
	if err := service.ProcessMessage(ctx, "devices/pk/d1/telemetry", []byte(`{"ts":1010000,"values":{"temperature":41}}`)); err != nil {
		t.Fatal(err)
	}
	alarms, _, err = store.ListAlarms(ctx, repository.AlarmFilter{DeviceID: "d1"})
	if err != nil || len(alarms) != 1 {
		t.Fatalf("alarms after duration=%#v err=%v", alarms, err)
	}
	if err := service.ProcessMessage(ctx, "devices/pk/d1/telemetry", []byte(`{"ts":1011000,"values":{"temperature":20}}`)); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessMessage(ctx, "devices/pk/d1/telemetry", []byte(`{"ts":1012000,"values":{"temperature":41}}`)); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessMessage(ctx, "devices/pk/d1/telemetry", []byte(`{"ts":1022000,"values":{"temperature":41}}`)); err != nil {
		t.Fatal(err)
	}
	alarms, _, err = store.ListAlarms(ctx, repository.AlarmFilter{ProductKey: "pk"})
	if err != nil || len(alarms) != 2 {
		t.Fatalf("alarms after reset=%#v err=%v", alarms, err)
	}
	if err := service.ProcessMessage(ctx, "devices/pk/d1/telemetry", []byte(`{"ts":1023000,"values":{"temperature":101}}`)); err == nil {
		t.Fatal("out-of-range telemetry should be rejected")
	}
}
