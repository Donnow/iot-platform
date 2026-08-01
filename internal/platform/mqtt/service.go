package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/messaging"
	"iot-perform/internal/platform/repository"
)

const (
	telemetrySubscription      = "$share/platform/devices/+/+/telemetry"
	eventSubscription          = "$share/platform/devices/+/+/event"
	commandReplySubscription   = "$share/platform/devices/+/+/command/reply"
	shadowReportedSubscription = "$share/platform/devices/+/+/shadow/reported"
)

type Config struct {
	BrokerURL string
	ClientID  string
	Username  string
	Password  string
}

type Client struct {
	client paho.Client
	lost   chan error
}

func NewClient(config Config) (*Client, error) {
	if strings.TrimSpace(config.BrokerURL) == "" {
		return nil, errors.New("MQTT broker URL is required")
	}
	if _, err := url.Parse(config.BrokerURL); err != nil {
		return nil, fmt.Errorf("invalid MQTT broker URL: %w", err)
	}
	lost := make(chan error, 1)
	options := paho.NewClientOptions()
	options.AddBroker(config.BrokerURL)
	options.SetClientID(config.ClientID)
	options.SetUsername(config.Username)
	options.SetPassword(config.Password)
	options.SetCleanSession(false)
	options.SetAutoReconnect(true)
	options.SetConnectionLostHandler(func(_ paho.Client, err error) {
		select {
		case lost <- err:
		default:
		}
	})
	return &Client{client: paho.NewClient(options), lost: lost}, nil
}

func (c *Client) Connect(ctx context.Context) error {
	token := c.client.Connect()
	if err := waitToken(ctx, token); err != nil {
		return err
	}
	return token.Error()
}

func (c *Client) Subscribe(ctx context.Context, topic string, handler func(string, []byte)) error {
	token := c.client.Subscribe(topic, messaging.QoSAtLeastOnce, func(_ paho.Client, message paho.Message) {
		handler(message.Topic(), append([]byte(nil), message.Payload()...))
	})
	if err := waitToken(ctx, token); err != nil {
		return err
	}
	return token.Error()
}

func (c *Client) Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error {
	token := c.client.Publish(topic, qos, retained, payload)
	if err := waitToken(ctx, token); err != nil {
		return err
	}
	return token.Error()
}

func (c *Client) Lost() <-chan error { return c.lost }

func (c *Client) Close(context.Context) error {
	c.client.Disconnect(250)
	return nil
}

type Service struct {
	repos   repository.Repositories
	client  *Client
	logger  *slog.Logger
	metrics Metrics
	rules   map[string]ruleState

	mu sync.Mutex
}

type ruleState struct {
	startedAt time.Time
	triggered bool
}

type Metrics interface {
	IncMQTTMessages()
	IncMQTTErrors()
	IncRuleMatches()
	IncAlarmsCreated()
}

func NewService(config Config, repos repository.Repositories, logger *slog.Logger) (*Service, error) {
	return NewServiceWithMetrics(config, repos, logger, nil)
}

func NewServiceWithMetrics(config Config, repos repository.Repositories, logger *slog.Logger, metrics Metrics) (*Service, error) {
	client, err := NewClient(config)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repos: repos, client: client, logger: logger, metrics: metrics, rules: make(map[string]ruleState)}, nil
}

func NewServiceWithClient(client *Client, repos repository.Repositories, logger *slog.Logger) *Service {
	return NewServiceWithClientAndMetrics(client, repos, logger, nil)
}

func NewServiceWithClientAndMetrics(client *Client, repos repository.Repositories, logger *slog.Logger, metrics Metrics) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repos: repos, client: client, logger: logger, metrics: metrics, rules: make(map[string]ruleState)}
}

func (s *Service) Start(ctx context.Context) error {
	if s.client == nil {
		return errors.New("MQTT client is required")
	}
	if err := s.client.Connect(ctx); err != nil {
		return err
	}
	for _, topic := range []string{telemetrySubscription, eventSubscription, commandReplySubscription, shadowReportedSubscription} {
		topic := topic
		if err := s.client.Subscribe(ctx, topic, func(topic string, payload []byte) {
			if err := s.ProcessMessage(context.Background(), topic, payload); err != nil {
				if s.metrics != nil {
					s.metrics.IncMQTTErrors()
				}
				s.logger.Error("process MQTT message", "topic", topic, "error", err)
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	if s.client == nil {
		return nil
	}
	return s.client.Close(ctx)
}

func (s *Service) Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error {
	if s.client == nil {
		return errors.New("MQTT client is required")
	}
	return s.client.Publish(ctx, topic, qos, retained, payload)
}

func (s *Service) ProcessMessage(ctx context.Context, topic string, payload []byte) error {
	if s.metrics != nil {
		s.metrics.IncMQTTMessages()
	}
	productKey, deviceID, suffix, err := ParseDeviceTopic(topic)
	if err != nil {
		return err
	}
	switch suffix {
	case "telemetry":
		var input struct {
			TS     int64          `json:"ts"`
			Values map[string]any `json:"values"`
		}
		if err := json.Unmarshal(payload, &input); err != nil || input.TS <= 0 || len(input.Values) == 0 {
			if err == nil {
				err = errors.New("telemetry requires ts and values")
			}
			return err
		}
		sample := domain.Telemetry{ProductKey: productKey, DeviceID: deviceID, Timestamp: time.UnixMilli(input.TS).UTC(), Values: input.Values}
		if s.repos.Products != nil {
			product, err := s.repos.Products.GetProductByKey(ctx, productKey)
			if err != nil {
				return err
			}
			if err := validateTelemetry(product, input.Values); err != nil {
				return err
			}
		}
		if err := s.repos.Telemetry.AppendTelemetry(ctx, sample); err != nil {
			return err
		}
		return s.evaluateRules(ctx, sample)
	case "event":
		return nil
	case "command/reply":
		var reply struct {
			CommandID string `json:"command_id"`
			Code      int    `json:"code"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal(payload, &reply); err != nil || reply.CommandID == "" {
			if err == nil {
				err = errors.New("command reply requires command_id")
			}
			return err
		}
		status := domain.CommandStatusSuccess
		if reply.Code != 0 {
			status = domain.CommandStatusFailed
		}
		return s.repos.Commands.UpdateCommandStatus(ctx, reply.CommandID, status, reply.Message, time.Now().UTC())
	case "shadow/reported":
		var input struct {
			Reported map[string]any `json:"reported"`
		}
		if err := json.Unmarshal(payload, &input); err != nil || input.Reported == nil {
			if err == nil {
				err = errors.New("shadow reported requires reported")
			}
			return err
		}
		_, err = s.repos.Shadows.UpsertReported(ctx, deviceID, input.Reported)
		return err
	default:
		return fmt.Errorf("unsupported device topic suffix %q", suffix)
	}
}

type ACLRule struct {
	Topic  string `json:"topic"`
	Action string `json:"action"`
}

type AuthResult struct {
	Allow bool      `json:"allow"`
	ACL   []ACLRule `json:"acl,omitempty"`
}

func (s *Service) Authenticate(ctx context.Context, deviceID, secret string) (AuthResult, error) {
	device, err := s.repos.Devices.AuthenticateDevice(ctx, deviceID, secret)
	if err != nil {
		return AuthResult{Allow: false}, nil
	}
	return AuthResult{Allow: true, ACL: []ACLRule{
		{Topic: messaging.DeviceTopic(device.ProductKey, deviceID, "telemetry"), Action: "publish"},
		{Topic: messaging.DeviceTopic(device.ProductKey, deviceID, "event"), Action: "publish"},
		{Topic: messaging.DeviceTopic(device.ProductKey, deviceID, "command/reply"), Action: "publish"},
		{Topic: messaging.DeviceTopic(device.ProductKey, deviceID, "shadow/reported"), Action: "publish"},
		{Topic: messaging.DeviceTopic(device.ProductKey, deviceID, "command"), Action: "subscribe"},
		{Topic: messaging.DeviceTopic(device.ProductKey, deviceID, "shadow/desired"), Action: "subscribe"},
		{Topic: messaging.DeviceTopic(device.ProductKey, deviceID, "ota"), Action: "subscribe"},
	}}, nil
}

func (s *Service) SetLifecycle(ctx context.Context, deviceID string, online bool, at time.Time) error {
	status := domain.DeviceStatusOffline
	if online {
		status = domain.DeviceStatusOnline
	}
	if err := s.repos.Devices.SetDeviceStatus(ctx, deviceID, status, &at); err != nil {
		return err
	}
	if !online || s.client == nil {
		return nil
	}
	shadow, err := s.repos.Shadows.GetShadow(ctx, deviceID)
	if err != nil || len(shadow.Delta) == 0 {
		return err
	}
	device, err := s.repos.Devices.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(shadow.Desired)
	if err != nil {
		return err
	}
	return s.Publish(ctx, messaging.DeviceTopic(device.ProductKey, deviceID, "shadow/desired"), messaging.QoSAtLeastOnce, false, payload)
}

func (s *Service) evaluateRules(ctx context.Context, sample domain.Telemetry) error {
	rules, err := s.repos.Rules.ListRulesByProduct(ctx, sample.ProductKey)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		matched := rule.Enabled && matches(rule.Operator, sample.Values[rule.PropertyName], rule.Threshold)
		if !s.shouldTrigger(sample.DeviceID, rule, matched, sample.Timestamp) {
			continue
		}
		if s.metrics != nil {
			s.metrics.IncRuleMatches()
		}
		value, ok := numberValue(sample.Values[rule.PropertyName])
		if !ok {
			continue
		}
		if rule.ActionType == "command" {
			if err := s.triggerRuleCommand(ctx, sample, rule); err != nil {
				return err
			}
			continue
		}
		if _, err := s.repos.Alarms.CreateAlarm(ctx, domain.Alarm{DeviceID: sample.DeviceID, RuleID: rule.ID, TriggerValue: value, TriggeredAt: sample.Timestamp}); err != nil {
			return err
		}
		if s.metrics != nil {
			s.metrics.IncAlarmsCreated()
		}
	}
	return nil
}

func (s *Service) triggerRuleCommand(ctx context.Context, sample domain.Telemetry, rule domain.Rule) error {
	method, _ := rule.ActionParams["method"].(string)
	if method == "" {
		return fmt.Errorf("rule %q command action requires method", rule.ID)
	}
	params := map[string]any{}
	if raw, ok := rule.ActionParams["params"].(map[string]any); ok {
		params = raw
	}
	device, err := s.repos.Devices.GetDevice(ctx, sample.DeviceID)
	if err != nil {
		return err
	}
	command, err := s.repos.Commands.CreateCommand(ctx, domain.Command{DeviceID: sample.DeviceID, Method: method, Params: params})
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"command_id": command.ID, "method": method, "params": params})
	if err != nil {
		return err
	}
	if err := s.Publish(ctx, messaging.DeviceTopic(device.ProductKey, sample.DeviceID, "command"), messaging.QoSAtLeastOnce, false, payload); err != nil {
		_ = s.repos.Commands.UpdateCommandStatus(ctx, command.ID, domain.CommandStatusFailed, err.Error(), time.Now().UTC())
		return err
	}
	return nil
}

func (s *Service) shouldTrigger(deviceID string, rule domain.Rule, matched bool, at time.Time) bool {
	key := deviceID + "\x00" + rule.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	if !matched {
		delete(s.rules, key)
		return false
	}
	state, exists := s.rules[key]
	if !exists {
		state = ruleState{startedAt: at}
	}
	if rule.DurationSeconds > 0 && at.Sub(state.startedAt) < time.Duration(rule.DurationSeconds)*time.Second {
		s.rules[key] = state
		return false
	}
	if state.triggered {
		return false
	}
	state.triggered = true
	s.rules[key] = state
	return true
}

func validateTelemetry(product domain.Product, values map[string]any) error {
	if len(product.Properties) == 0 {
		return nil
	}
	properties := make(map[string]domain.Property, len(product.Properties))
	for _, property := range product.Properties {
		properties[property.Name] = property
	}
	for name, raw := range values {
		property, ok := properties[name]
		if !ok {
			return fmt.Errorf("telemetry property %q is not defined", name)
		}
		switch property.DataType {
		case domain.PropertyTypeInt:
			value, ok := numberValue(raw)
			if !ok || value != float64(int64(value)) {
				return fmt.Errorf("telemetry property %q must be an integer", name)
			}
		case domain.PropertyTypeFloat:
			if _, ok := numberValue(raw); !ok {
				return fmt.Errorf("telemetry property %q must be a number", name)
			}
		case domain.PropertyTypeBool:
			if _, ok := raw.(bool); !ok {
				return fmt.Errorf("telemetry property %q must be a boolean", name)
			}
		case domain.PropertyTypeString:
			if _, ok := raw.(string); !ok {
				return fmt.Errorf("telemetry property %q must be a string", name)
			}
		default:
			return fmt.Errorf("telemetry property %q has unsupported type %q", name, property.DataType)
		}
		if value, ok := numberValue(raw); ok {
			if property.MinValue != nil && value < *property.MinValue {
				return fmt.Errorf("telemetry property %q is below minimum", name)
			}
			if property.MaxValue != nil && value > *property.MaxValue {
				return fmt.Errorf("telemetry property %q is above maximum", name)
			}
		}
	}
	return nil
}

func ParseDeviceTopic(topic string) (productKey, deviceID, suffix string, err error) {
	parts := strings.Split(strings.Trim(topic, "/"), "/")
	if len(parts) < 4 || parts[0] != "devices" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid device topic %q", topic)
	}
	return parts[1], parts[2], strings.Join(parts[3:], "/"), nil
}

func matches(operator string, raw any, threshold float64) bool {
	value, ok := numberValue(raw)
	if !ok {
		return false
	}
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

func numberValue(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	default:
		return 0, false
	}
}

func waitToken(ctx context.Context, token paho.Token) error {
	if token.WaitTimeout(15 * time.Second) {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("MQTT operation timed out")
}
