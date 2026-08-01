package domain

import "time"

type DeviceType string

const (
	DeviceTypeSensor    DeviceType = "sensor"
	DeviceTypeActuator  DeviceType = "actuator"
	DeviceTypeComposite DeviceType = "composite"
)

type PropertyType string

const (
	PropertyTypeInt    PropertyType = "int"
	PropertyTypeFloat  PropertyType = "float"
	PropertyTypeBool   PropertyType = "bool"
	PropertyTypeString PropertyType = "string"
)

type DeviceStatus string

const (
	DeviceStatusInactive DeviceStatus = "inactive"
	DeviceStatusOnline   DeviceStatus = "online"
	DeviceStatusOffline  DeviceStatus = "offline"
	DeviceStatusDeleted  DeviceStatus = "deleted"
)

type AlarmStatus string

const (
	AlarmStatusActive   AlarmStatus = "active"
	AlarmStatusResolved AlarmStatus = "resolved"
)

type CommandStatus string

const (
	CommandStatusPending CommandStatus = "pending"
	CommandStatusSuccess CommandStatus = "success"
	CommandStatusFailed  CommandStatus = "failed"
	CommandStatusTimeout CommandStatus = "timeout"
)

type Product struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	ProductKey        string     `json:"product_key"`
	Description       string     `json:"description,omitempty"`
	DeviceType        DeviceType `json:"device_type"`
	Properties        []Property `json:"properties,omitempty"`
	OnlineDeviceCount int        `json:"online_device_count"`
	CreatedAt         time.Time  `json:"created_at"`
}

type Property struct {
	Name     string       `json:"name"`
	DataType PropertyType `json:"data_type"`
	Unit     string       `json:"unit,omitempty"`
	MinValue *float64     `json:"min_value,omitempty"`
	MaxValue *float64     `json:"max_value,omitempty"`
}

type Device struct {
	ID           string       `json:"id"`
	DeviceID     string       `json:"device_id"`
	DeviceSecret string       `json:"-"`
	ProductKey   string       `json:"product_key"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Status       DeviceStatus `json:"status"`
	LastOnline   *time.Time   `json:"last_online,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type Telemetry struct {
	DeviceID   string         `json:"device_id"`
	ProductKey string         `json:"product_key"`
	Timestamp  time.Time      `json:"timestamp"`
	Values     map[string]any `json:"values"`
}

type Rule struct {
	ID              string         `json:"id"`
	ProductKey      string         `json:"product_key"`
	Name            string         `json:"name"`
	PropertyName    string         `json:"property_name"`
	Operator        string         `json:"operator"`
	Threshold       float64        `json:"threshold"`
	DurationSeconds int            `json:"duration_seconds"`
	ActionType      string         `json:"action_type"`
	ActionParams    map[string]any `json:"action_params,omitempty"`
	Enabled         bool           `json:"enabled"`
	CreatedAt       time.Time      `json:"created_at"`
}

type Alarm struct {
	ID           string      `json:"id"`
	DeviceID     string      `json:"device_id"`
	RuleID       string      `json:"rule_id"`
	TriggerValue float64     `json:"trigger_value"`
	Status       AlarmStatus `json:"status"`
	TriggeredAt  time.Time   `json:"triggered_at"`
	ResolvedAt   *time.Time  `json:"resolved_at,omitempty"`
	ResolveNote  string      `json:"resolve_note,omitempty"`
}

type Command struct {
	ID        string         `json:"command_id"`
	DeviceID  string         `json:"device_id"`
	Method    string         `json:"method"`
	Params    map[string]any `json:"params,omitempty"`
	Status    CommandStatus  `json:"status"`
	Message   string         `json:"message,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Shadow struct {
	DeviceID  string         `json:"device_id"`
	Desired   map[string]any `json:"desired"`
	Reported  map[string]any `json:"reported"`
	Delta     map[string]any `json:"delta"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Firmware struct {
	ID         string    `json:"id"`
	ProductKey string    `json:"product_key"`
	Version    string    `json:"version"`
	MD5        string    `json:"md5"`
	FileURL    string    `json:"file_url"`
	Changelog  string    `json:"changelog,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type OTAStage string

const (
	OTAStagePending     OTAStage = "pending"
	OTAStageDownloading OTAStage = "downloading"
	OTAStageInstalling  OTAStage = "installing"
	OTAStageSuccess     OTAStage = "success"
	OTAStageFailed      OTAStage = "failed"
)

type OTADeviceProgress struct {
	DeviceID  string    `json:"device_id"`
	Stage     OTAStage  `json:"stage"`
	Progress  int       `json:"progress"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OTATask struct {
	ID              string              `json:"task_id"`
	ProductKey      string              `json:"product_key"`
	FirmwareID      string              `json:"firmware_id"`
	Version         string              `json:"version"`
	URL             string              `json:"url"`
	MD5             string              `json:"md5"`
	TargetDeviceIDs []string            `json:"target_device_ids"`
	Progress        []OTADeviceProgress `json:"progress"`
	Summary         map[OTAStage]int    `json:"summary"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}
