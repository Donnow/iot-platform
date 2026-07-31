package repository

import (
	"context"
	"time"

	"iot-perform/internal/platform/domain"
)

type Page struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

type ProductFilter struct {
	Page     int
	PageSize int
}

type DeviceFilter struct {
	ProductKey string
	Status     domain.DeviceStatus
	Page       int
	PageSize   int
}

type TelemetryQuery struct {
	DeviceID string
	Metric   string
	From     time.Time
	To       time.Time
	Limit    int
}

type AlarmFilter struct {
	DeviceID   string
	ProductKey string
	Status     domain.AlarmStatus
	From       time.Time
	To         time.Time
	Page       int
	PageSize   int
}

type ProductRepository interface {
	CreateProduct(context.Context, domain.Product) (domain.Product, error)
	GetProductByKey(context.Context, string) (domain.Product, error)
	ListProducts(context.Context, ProductFilter) ([]domain.Product, Page, error)
}

type DeviceRepository interface {
	CreateDevice(context.Context, domain.Device) (domain.Device, error)
	GetDevice(context.Context, string) (domain.Device, error)
	ListDevices(context.Context, DeviceFilter) ([]domain.Device, Page, error)
	SetDeviceStatus(context.Context, string, domain.DeviceStatus, *time.Time) error
	SoftDeleteDevice(context.Context, string) error
	AuthenticateDevice(context.Context, string, string) (domain.Device, error)
}

type TelemetryRepository interface {
	AppendTelemetry(context.Context, domain.Telemetry) error
	QueryTelemetry(context.Context, TelemetryQuery) ([]domain.Telemetry, error)
	SnapshotTelemetry(context.Context, string) (map[string]domain.Telemetry, error)
}

type RuleRepository interface {
	CreateRule(context.Context, domain.Rule) (domain.Rule, error)
	ListRulesByProduct(context.Context, string) ([]domain.Rule, error)
}

type AlarmRepository interface {
	CreateAlarm(context.Context, domain.Alarm) (domain.Alarm, error)
	GetAlarm(context.Context, string) (domain.Alarm, error)
	ListAlarms(context.Context, AlarmFilter) ([]domain.Alarm, Page, error)
	ResolveAlarm(context.Context, string, time.Time, string) error
}

type CommandRepository interface {
	CreateCommand(context.Context, domain.Command) (domain.Command, error)
	GetCommand(context.Context, string, string) (domain.Command, error)
	UpdateCommandStatus(context.Context, string, domain.CommandStatus, string, time.Time) error
}

type ShadowRepository interface {
	GetShadow(context.Context, string) (domain.Shadow, error)
	UpsertDesired(context.Context, string, map[string]any) (domain.Shadow, error)
	UpsertReported(context.Context, string, map[string]any) (domain.Shadow, error)
}

type Repositories struct {
	Products  ProductRepository
	Devices   DeviceRepository
	Telemetry TelemetryRepository
	Rules     RuleRepository
	Alarms    AlarmRepository
	Commands  CommandRepository
	Shadows   ShadowRepository
}
