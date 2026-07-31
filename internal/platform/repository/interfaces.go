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
	Create(context.Context, domain.Product) (domain.Product, error)
	GetByKey(context.Context, string) (domain.Product, error)
	List(context.Context, ProductFilter) ([]domain.Product, Page, error)
}

type DeviceRepository interface {
	Create(context.Context, domain.Device) (domain.Device, error)
	Get(context.Context, string) (domain.Device, error)
	List(context.Context, DeviceFilter) ([]domain.Device, Page, error)
	SetStatus(context.Context, string, domain.DeviceStatus, *time.Time) error
	SoftDelete(context.Context, string) error
	Authenticate(context.Context, string, string) (domain.Device, error)
}

type TelemetryRepository interface {
	Append(context.Context, domain.Telemetry) error
	Query(context.Context, TelemetryQuery) ([]domain.Telemetry, error)
	Snapshot(context.Context, string) (map[string]domain.Telemetry, error)
}

type RuleRepository interface {
	Create(context.Context, domain.Rule) (domain.Rule, error)
	ListByProduct(context.Context, string) ([]domain.Rule, error)
}

type AlarmRepository interface {
	Create(context.Context, domain.Alarm) (domain.Alarm, error)
	Get(context.Context, string) (domain.Alarm, error)
	List(context.Context, AlarmFilter) ([]domain.Alarm, Page, error)
	Resolve(context.Context, string, time.Time, string) error
}

type CommandRepository interface {
	Create(context.Context, domain.Command) (domain.Command, error)
	Get(context.Context, string, string) (domain.Command, error)
	UpdateStatus(context.Context, string, domain.CommandStatus, string, time.Time) error
}

type ShadowRepository interface {
	Get(context.Context, string) (domain.Shadow, error)
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
