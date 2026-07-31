package devicesim

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func testConfig(deviceType DeviceType) Config {
	config := DefaultConfig()
	config.ProductKey = "pk"
	config.DeviceType = deviceType
	config.Count = 1
	config.Interval = time.Second
	return config
}

func newTestDevice(t *testing.T, deviceType DeviceType) (*Device, *fakeFactory, *fakeClock) {
	t.Helper()
	factory := &fakeFactory{}
	clock := newFakeClock()
	device, err := NewDevice(testConfig(deviceType), DeviceCredential{DeviceID: "device-1", DeviceSecret: "secret-1"}, 0, factory.Create, DeviceOptions{
		Clock:      clock,
		Random:     &fixedRandom{values: []float64{0.5}},
		BackoffMin: time.Nanosecond,
		BackoffMax: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return device, factory, clock
}

func TestConnectionOptionsAndSubscriptions(t *testing.T) {
	device, factory, _ := newTestDevice(t, DeviceTypeTemperature)
	options := device.ConnectionOptions()
	if options.BrokerURL != "tcp://localhost:1883" || options.ClientID != "device-1" || options.Username != "device-1" || options.Password != "secret-1" {
		t.Fatalf("connection options = %#v", options)
	}
	if options.WillTopic != "devices/pk/device-1/event" || string(options.WillPayload) != `{"status":"offline"}` || options.WillQoS != QoSAtLeastOnce {
		t.Fatalf("will options = %#v", options)
	}
	if err := device.ConnectOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	subscriptions := factory.Clients()[0].Subscriptions()
	if len(subscriptions) != 3 {
		t.Fatalf("got %d subscriptions, want 3", len(subscriptions))
	}
	for _, subscription := range subscriptions {
		if subscription.QoS != QoSAtLeastOnce {
			t.Fatalf("subscription %#v has wrong QoS", subscription)
		}
	}
}

func TestPublishTelemetryUsesTopicQoSAndEnvelope(t *testing.T) {
	device, factory, _ := newTestDevice(t, DeviceTypeTemperature)
	if err := device.ConnectOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := factory.Clients()[0]
	now := time.UnixMilli(1722000000000)
	if err := device.PublishTelemetry(context.Background(), client, now); err != nil {
		t.Fatal(err)
	}
	messages := client.Published()
	if len(messages) != 1 || messages[0].Topic != "devices/pk/device-1/telemetry" || messages[0].QoS != QoSAtLeastOnce {
		t.Fatalf("published messages = %#v", messages)
	}
	var payload TelemetryPayload
	if err := json.Unmarshal(messages[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TS != now.UnixMilli() || payload.Values["temperature"] == nil || payload.Values["humidity"] == nil {
		t.Fatalf("telemetry payload = %#v", payload)
	}
}

func TestCommandIsIdempotentAndShadowReported(t *testing.T) {
	device, factory, _ := newTestDevice(t, DeviceTypeDoor)
	if err := device.ConnectOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := factory.Clients()[0]
	commandTopic := Topic("pk", "device-1", topicCommand)
	openPayload := mustJSON(t, Command{CommandID: "c1", Method: "open"})
	if err := client.Deliver(commandTopic, openPayload); err != nil {
		t.Fatal(err)
	}
	if err := client.Deliver(commandTopic, mustJSON(t, Command{CommandID: "c1", Method: "close"})); err != nil {
		t.Fatal(err)
	}
	behavior := device.behavior.(*DoorBehavior)
	if behavior.status != "open" {
		t.Fatalf("duplicate command changed state to %q", behavior.status)
	}
	messages := client.Published()
	if len(messages) != 3 {
		t.Fatalf("published command messages = %#v", messages)
	}

	shadowTopic := Topic("pk", "device-1", topicShadowDesired)
	if err := client.Deliver(shadowTopic, []byte(`{"target":"ignored"}`)); err != nil {
		t.Fatal(err)
	}
	messages = client.Published()
	last := messages[len(messages)-1]
	if last.Topic != "devices/pk/device-1/shadow/reported" {
		t.Fatalf("last message = %#v", last)
	}
}

func TestOTAIsIdempotent(t *testing.T) {
	device, factory, _ := newTestDevice(t, DeviceTypeDoor)
	if err := device.ConnectOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := factory.Clients()[0]
	request := OTARequest{Version: "1.2.0", URL: "https://example.test/fw.bin", MD5: "0123456789abcdef0123456789abcdef"}
	payload := mustJSON(t, request)
	if err := client.Deliver(Topic("pk", "device-1", topicOTA), payload); err != nil {
		t.Fatal(err)
	}
	if err := client.Deliver(Topic("pk", "device-1", topicOTA), payload); err != nil {
		t.Fatal(err)
	}
	messages := client.Published()
	if len(messages) != 3 {
		t.Fatalf("OTA messages = %#v", messages)
	}
}

func TestRunReconnectsAndStops(t *testing.T) {
	device, factory, _ := newTestDevice(t, DeviceTypeTemperature)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- device.Run(ctx) }()
	waitFor(t, time.Second, func() bool { return len(factory.Clients()) >= 1 })
	factory.Clients()[0].lost <- errors.New("connection lost")
	waitFor(t, time.Second, func() bool { return len(factory.Clients()) >= 2 })
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition was not met within %s", timeout)
}

func TestHandleMessageIgnoresOtherDeviceTopic(t *testing.T) {
	device, factory, _ := newTestDevice(t, DeviceTypeDoor)
	if err := device.ConnectOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := factory.Clients()[0]
	if err := device.HandleMessage(context.Background(), client, Message{
		Topic:   Topic("pk", "other-device", topicCommand),
		Payload: mustJSON(t, Command{CommandID: "c1", Method: "open"}),
	}); err != nil {
		t.Fatal(err)
	}
	if len(client.Published()) != 0 {
		t.Fatal("message for another device must not be handled")
	}
}

func TestMalformedCommandProducesFailureReply(t *testing.T) {
	device, factory, _ := newTestDevice(t, DeviceTypeDoor)
	if err := device.ConnectOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := factory.Clients()[0]
	if err := client.Deliver(Topic("pk", "device-1", topicCommand), []byte("{")); err != nil {
		t.Fatal(err)
	}
	messages := client.Published()
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	var reply CommandReply
	if err := json.Unmarshal(messages[0].Payload, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Code == 0 || reply.Message == "" {
		t.Fatalf("reply = %#v", reply)
	}
}

func TestFakeClientContextError(t *testing.T) {
	client := newFakeClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Connect(ctx); err == nil {
		t.Fatal("cancelled connect should fail")
	}
	if err := client.Publish(ctx, "topic", 1, false, nil); err == nil {
		t.Fatal("cancelled publish should fail")
	}
}
