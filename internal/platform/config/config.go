package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	StorageMode              string
	HTTPAddr                 string
	MQTTBrokerURL            string
	MQTTClientID             string
	MQTTUsername             string
	MQTTPassword             string
	DatabaseURL              string
	RedisAddr                string
	RedisPassword            string
	RedisDB                  int
	TDengineURL              string
	TDengineUser             string
	TDenginePassword         string
	MQTTWorkers              int
	MQTTQueueSize            int
	TelemetryBatchSize       int
	TelemetryBatchIntervalMS int
	ProductCacheTTLSeconds   int
	JWTSecret                string
	JWTTTLSeconds            int
	AdminUsername            string
	AdminPassword            string
	RequestTimeout           int
}

func Default() Config {
	return Config{
		StorageMode:              "persistent",
		HTTPAddr:                 ":8080",
		MQTTBrokerURL:            "tcp://localhost:1883",
		MQTTClientID:             "iot-platform",
		DatabaseURL:              "postgres://iot:iot@localhost:5432/iot?sslmode=disable",
		RedisAddr:                "localhost:6379",
		RedisDB:                  0,
		TDengineURL:              "http://localhost:6041",
		MQTTWorkers:              4,
		MQTTQueueSize:            1024,
		TelemetryBatchSize:       200,
		TelemetryBatchIntervalMS: 200,
		ProductCacheTTLSeconds:   60,
		JWTSecret:                "change-me-in-production",
		JWTTTLSeconds:            3600,
		AdminUsername:            "admin",
		AdminPassword:            "admin123456",
		RequestTimeout:           10,
	}
}

func FromEnv() Config {
	c := Default()
	if value := os.Getenv("IOT_STORAGE_MODE"); value != "" {
		c.StorageMode = strings.ToLower(strings.TrimSpace(value))
	}
	if value := os.Getenv("IOT_HTTP_ADDR"); value != "" {
		c.HTTPAddr = value
	}
	if value := os.Getenv("IOT_MQTT_BROKER_URL"); value != "" {
		c.MQTTBrokerURL = value
	}
	if value := os.Getenv("IOT_MQTT_CLIENT_ID"); value != "" {
		c.MQTTClientID = value
	}
	if value := os.Getenv("IOT_MQTT_USERNAME"); value != "" {
		c.MQTTUsername = value
	}
	if value := os.Getenv("IOT_MQTT_PASSWORD"); value != "" {
		c.MQTTPassword = value
	}
	if value := os.Getenv("IOT_DATABASE_URL"); value != "" {
		c.DatabaseURL = value
	}
	if value := os.Getenv("IOT_REDIS_ADDR"); value != "" {
		c.RedisAddr = value
	}
	if value := os.Getenv("IOT_REDIS_PASSWORD"); value != "" {
		c.RedisPassword = value
	}
	if value := os.Getenv("IOT_REDIS_DB"); value != "" {
		if db, err := strconv.Atoi(value); err == nil {
			c.RedisDB = db
		}
	}
	if value := os.Getenv("IOT_TDENGINE_URL"); value != "" {
		c.TDengineURL = value
	}
	if value := os.Getenv("IOT_TDENGINE_USERNAME"); value != "" {
		c.TDengineUser = value
	}
	if value := os.Getenv("IOT_TDENGINE_PASSWORD"); value != "" {
		c.TDenginePassword = value
	}
	if value := os.Getenv("IOT_MQTT_WORKERS"); value != "" {
		if workers, err := strconv.Atoi(value); err == nil {
			c.MQTTWorkers = workers
		}
	}
	if value := os.Getenv("IOT_MQTT_QUEUE_SIZE"); value != "" {
		if size, err := strconv.Atoi(value); err == nil {
			c.MQTTQueueSize = size
		}
	}
	if value := os.Getenv("IOT_TELEMETRY_BATCH_SIZE"); value != "" {
		if size, err := strconv.Atoi(value); err == nil {
			c.TelemetryBatchSize = size
		}
	}
	if value := os.Getenv("IOT_TELEMETRY_BATCH_INTERVAL_MS"); value != "" {
		if interval, err := strconv.Atoi(value); err == nil {
			c.TelemetryBatchIntervalMS = interval
		}
	}
	if value := os.Getenv("IOT_PRODUCT_CACHE_TTL_SECONDS"); value != "" {
		if ttl, err := strconv.Atoi(value); err == nil {
			c.ProductCacheTTLSeconds = ttl
		}
	}
	if value := os.Getenv("IOT_JWT_SECRET"); value != "" {
		c.JWTSecret = value
	}
	if value := os.Getenv("IOT_JWT_TTL_SECONDS"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			c.JWTTTLSeconds = seconds
		}
	}
	if value := os.Getenv("IOT_ADMIN_USERNAME"); value != "" {
		c.AdminUsername = value
	}
	if value := os.Getenv("IOT_ADMIN_PASSWORD"); value != "" {
		c.AdminPassword = value
	}
	if value := os.Getenv("IOT_REQUEST_TIMEOUT_SECONDS"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			c.RequestTimeout = seconds
		}
	}
	return c
}

func (c Config) Validate() error {
	if c.StorageMode != "memory" && c.StorageMode != "persistent" {
		return errors.New("storage mode must be memory or persistent")
	}
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return errors.New("HTTP address is required")
	}
	if strings.TrimSpace(c.MQTTBrokerURL) == "" {
		return errors.New("MQTT broker URL is required")
	}
	if strings.TrimSpace(c.MQTTClientID) == "" {
		return errors.New("MQTT client ID is required")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("database URL is required")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return errors.New("Redis address is required")
	}
	if c.RedisDB < 0 {
		return errors.New("Redis DB must not be negative")
	}
	if strings.TrimSpace(c.TDengineURL) == "" {
		return errors.New("TDengine URL is required")
	}
	if c.MQTTWorkers <= 0 {
		return errors.New("MQTT workers must be positive")
	}
	if c.MQTTQueueSize <= 0 {
		return errors.New("MQTT queue size must be positive")
	}
	if c.TelemetryBatchSize <= 0 {
		return errors.New("telemetry batch size must be positive")
	}
	if c.TelemetryBatchIntervalMS <= 0 {
		return errors.New("telemetry batch interval must be positive")
	}
	if c.ProductCacheTTLSeconds <= 0 {
		return errors.New("product cache TTL must be positive")
	}
	if len(c.JWTSecret) < 16 {
		return fmt.Errorf("JWT secret must be at least 16 characters")
	}
	if c.JWTTTLSeconds <= 0 {
		return fmt.Errorf("JWT TTL must be positive")
	}
	if c.AdminUsername == "" || c.AdminPassword == "" {
		return fmt.Errorf("admin username and password are required")
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be greater than zero")
	}
	return nil
}
