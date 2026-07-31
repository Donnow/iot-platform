package devicesim

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	QoSAtLeastOnce byte = 1

	topicTelemetry      = "telemetry"
	topicEvent          = "event"
	topicCommand        = "command"
	topicCommandReply   = "command/reply"
	topicShadowDesired  = "shadow/desired"
	topicShadowReported = "shadow/reported"
	topicOTA            = "ota"
)

type Message struct {
	Topic   string
	Payload []byte
}

type MessageHandler func(Message)

type MQTTOptions struct {
	BrokerURL    string
	ClientID     string
	Username     string
	Password     string
	WillTopic    string
	WillPayload  []byte
	WillQoS      byte
	WillRetained bool
}

type Client interface {
	Connect(context.Context) error
	Subscribe(context.Context, string, byte, MessageHandler) error
	Publish(context.Context, string, byte, bool, []byte) error
	Lost() <-chan error
	Disconnect(context.Context) error
}

type ClientFactory func(MQTTOptions) (Client, error)

type TelemetryPayload struct {
	TS     int64          `json:"ts"`
	Values map[string]any `json:"values"`
}

type EventPayload struct {
	TS        int64          `json:"ts"`
	EventType string         `json:"event_type"`
	Data      map[string]any `json:"data"`
}

type Command struct {
	CommandID string         `json:"command_id"`
	Method    string         `json:"method"`
	Params    map[string]any `json:"params"`
}

type CommandReply struct {
	CommandID string `json:"command_id"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
}

type OTARequest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	MD5     string `json:"md5"`
}

type ShadowReported struct {
	TS       int64          `json:"ts"`
	Reported map[string]any `json:"reported"`
}

type Event struct {
	EventType string
	Data      map[string]any
}

func Topic(productKey, deviceID, suffix string) string {
	return fmt.Sprintf("devices/%s/%s/%s", productKey, deviceID, suffix)
}

func encodeJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func decodeJSON(payload []byte, target any) error {
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode JSON payload: %w", err)
	}
	return nil
}
