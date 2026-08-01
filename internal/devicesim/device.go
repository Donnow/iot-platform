package devicesim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type DeviceOptions struct {
	Clock      Clock
	Random     Random
	Logger     *slog.Logger
	BackoffMin time.Duration
	BackoffMax time.Duration
}

type Device struct {
	productKey string
	brokerURL  string
	credential DeviceCredential
	deviceType DeviceType
	interval   time.Duration
	factory    ClientFactory
	clock      Clock
	behavior   Behavior
	random     Random
	logger     *slog.Logger
	backoffMin time.Duration
	backoffMax time.Duration

	mu            sync.Mutex
	client        Client
	processed     map[string]CommandReply
	otaVersions   map[string]struct{}
	desiredShadow map[string]any
}

func NewDevice(config Config, credential DeviceCredential, index int, factory ClientFactory, options DeviceOptions) (*Device, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := credential.Validate(); err != nil {
		return nil, err
	}
	if factory == nil {
		return nil, errors.New("MQTT client factory is required")
	}
	behavior, err := buildBehavior(config)
	if err != nil {
		return nil, err
	}
	clock := options.Clock
	if clock == nil {
		clock = realClock{}
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	random := options.Random
	if random == nil {
		random = newRandom(config.Seed, index)
	}
	backoffMin := options.BackoffMin
	if backoffMin <= 0 {
		backoffMin = time.Second
	}
	backoffMax := options.BackoffMax
	if backoffMax <= 0 {
		backoffMax = 30 * time.Second
	}
	if backoffMax < backoffMin {
		return nil, errors.New("backoff max must be greater than or equal to backoff min")
	}
	return &Device{
		productKey:    config.ProductKey,
		brokerURL:     config.BrokerURL,
		credential:    credential,
		deviceType:    config.DeviceType,
		interval:      config.Interval,
		factory:       factory,
		clock:         clock,
		behavior:      behavior,
		random:        random,
		logger:        logger,
		backoffMin:    backoffMin,
		backoffMax:    backoffMax,
		processed:     make(map[string]CommandReply),
		otaVersions:   make(map[string]struct{}),
		desiredShadow: make(map[string]any),
	}, nil
}

func (d *Device) DeviceID() string {
	return d.credential.DeviceID
}

func (d *Device) DeviceType() DeviceType {
	return d.deviceType
}

func (d *Device) ConnectionOptions() MQTTOptions {
	willPayload := fmt.Sprintf(`{"status":"offline","ts":%d}`, d.clock.Now().UnixMilli())
	return MQTTOptions{
		BrokerURL:    d.brokerURL,
		ClientID:     d.credential.DeviceID,
		Username:     d.credential.DeviceID,
		Password:     d.credential.DeviceSecret,
		WillTopic:    Topic(d.productKey, d.credential.DeviceID, topicEvent),
		WillPayload:  []byte(willPayload),
		WillQoS:      QoSAtLeastOnce,
		WillRetained: false,
	}
}

func (d *Device) ConnectOnce(ctx context.Context) error {
	d.mu.Lock()
	if d.client != nil {
		d.mu.Unlock()
		return errors.New("device is already connected")
	}
	d.mu.Unlock()

	client, err := d.factory(d.ConnectionOptions())
	if err != nil {
		return fmt.Errorf("create MQTT client: %w", err)
	}
	if client == nil {
		return errors.New("MQTT client factory returned nil client")
	}
	if err := client.Connect(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return fmt.Errorf("connect device %s: %w", d.DeviceID(), err)
	}
	for _, subscription := range d.subscriptions() {
		subscription := subscription
		if err := client.Subscribe(ctx, subscription.topic, QoSAtLeastOnce, func(message Message) {
			if err := d.HandleMessage(context.Background(), client, message); err != nil {
				d.logger.Error("handle device message", "device_id", d.DeviceID(), "topic", message.Topic, "error", err)
			}
		}); err != nil {
			_ = client.Disconnect(context.Background())
			return fmt.Errorf("subscribe %s: %w", subscription.topic, err)
		}
	}
	d.mu.Lock()
	d.client = client
	d.mu.Unlock()
	return nil
}

func (d *Device) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	client := d.client
	d.client = nil
	d.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Disconnect(ctx)
}

func (d *Device) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	backoff := d.backoffMin
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := d.ConnectOnce(ctx); err != nil {
			d.logger.Error("device connection failed", "device_id", d.DeviceID(), "error", err)
			if err := d.waitBackoff(ctx, backoff); err != nil {
				return nil
			}
			backoff = minDuration(backoff*2, d.backoffMax)
			continue
		}
		backoff = d.backoffMin
		if err := d.runConnected(ctx); err != nil && ctx.Err() == nil {
			d.logger.Warn("device connection lost", "device_id", d.DeviceID(), "error", err)
			if err := d.waitBackoff(ctx, backoff); err != nil {
				return nil
			}
			backoff = minDuration(backoff*2, d.backoffMax)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func (d *Device) runConnected(ctx context.Context) error {
	d.mu.Lock()
	client := d.client
	d.mu.Unlock()
	if client == nil {
		return errors.New("device is not connected")
	}
	ticker := d.clock.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return d.Disconnect(ctx)
		case err, ok := <-client.Lost():
			_ = d.Disconnect(context.Background())
			if !ok || err == nil {
				return errors.New("MQTT connection lost")
			}
			return err
		case now := <-ticker.Chan():
			if err := d.PublishTelemetry(ctx, client, now); err != nil {
				_ = d.Disconnect(context.Background())
				return err
			}
		}
	}
}

func (d *Device) PublishTelemetry(ctx context.Context, client Client, now time.Time) error {
	d.mu.Lock()
	values, events := d.behavior.Tick(d.random)
	d.mu.Unlock()
	if err := d.publish(ctx, client, topicTelemetry, TelemetryPayload{TS: now.UnixMilli(), Values: values}); err != nil {
		return err
	}
	for _, event := range events {
		if err := d.publishEvent(ctx, client, now, event); err != nil {
			return err
		}
	}
	return nil
}

func (d *Device) HandleMessage(ctx context.Context, client Client, message Message) error {
	if client == nil {
		return errors.New("MQTT client is required")
	}
	switch message.Topic {
	case Topic(d.productKey, d.DeviceID(), topicCommand):
		return d.handleCommand(ctx, client, message.Payload)
	case Topic(d.productKey, d.DeviceID(), topicShadowDesired):
		return d.handleShadow(ctx, client, message.Payload)
	case Topic(d.productKey, d.DeviceID(), topicOTA):
		return d.handleOTA(ctx, client, message.Payload)
	default:
		return nil
	}
}

func (d *Device) handleCommand(ctx context.Context, client Client, payload []byte) error {
	var command Command
	if err := decodeJSON(payload, &command); err != nil {
		return d.publish(ctx, client, topicCommandReply, failedCommand("", "invalid command payload"))
	}
	d.mu.Lock()
	if cached, ok := d.processed[command.CommandID]; ok && command.CommandID != "" {
		d.mu.Unlock()
		return d.publish(ctx, client, topicCommandReply, cached)
	}
	reply, telemetry := d.behavior.HandleCommand(command)
	if command.CommandID != "" {
		d.processed[command.CommandID] = reply
	}
	d.mu.Unlock()
	if err := d.publish(ctx, client, topicCommandReply, reply); err != nil {
		return err
	}
	if telemetry != nil {
		return d.publish(ctx, client, topicTelemetry, TelemetryPayload{TS: d.clock.Now().UnixMilli(), Values: telemetry})
	}
	return nil
}

func (d *Device) handleShadow(ctx context.Context, client Client, payload []byte) error {
	var desired map[string]any
	if err := decodeJSON(payload, &desired); err != nil {
		return err
	}
	if wrapped, ok := desired["desired"].(map[string]any); ok {
		desired = wrapped
	}
	d.mu.Lock()
	reported, err := d.behavior.HandleShadow(desired)
	if err == nil {
		d.desiredShadow = cloneMap(desired)
	}
	d.mu.Unlock()
	if err != nil {
		return err
	}
	return d.publish(ctx, client, topicShadowReported, ShadowReported{
		TS:       d.clock.Now().UnixMilli(),
		Reported: reported,
	})
}

func (d *Device) handleOTA(ctx context.Context, client Client, payload []byte) error {
	var request OTARequest
	if err := decodeJSON(payload, &request); err != nil {
		return err
	}
	d.mu.Lock()
	if _, exists := d.otaVersions[request.Version]; exists && request.Version != "" {
		d.mu.Unlock()
		return nil
	}
	events := d.behavior.HandleOTA(request)
	if validOTARequest(request) {
		d.otaVersions[request.Version] = struct{}{}
	}
	d.mu.Unlock()
	now := d.clock.Now()
	for _, event := range events {
		if err := d.publishEvent(ctx, client, now, event); err != nil {
			return err
		}
	}
	return nil
}

func (d *Device) publish(ctx context.Context, client Client, suffix string, payload any) error {
	data, err := encodeJSON(payload)
	if err != nil {
		return err
	}
	return client.Publish(ctx, Topic(d.productKey, d.DeviceID(), suffix), QoSAtLeastOnce, false, data)
}

func (d *Device) publishEvent(ctx context.Context, client Client, now time.Time, event Event) error {
	return d.publish(ctx, client, topicEvent, EventPayload{TS: now.UnixMilli(), EventType: event.EventType, Data: event.Data})
}

type subscription struct {
	topic string
}

func (d *Device) subscriptions() []subscription {
	return []subscription{
		{topic: Topic(d.productKey, d.DeviceID(), topicCommand)},
		{topic: Topic(d.productKey, d.DeviceID(), topicShadowDesired)},
		{topic: Topic(d.productKey, d.DeviceID(), topicOTA)},
	}
}

func (d *Device) waitBackoff(ctx context.Context, backoff time.Duration) error {
	if backoff <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.clock.After(backoff):
		return nil
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
