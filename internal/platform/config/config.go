package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	StorageMode      string
	HTTPAddr         string
	MQTTBrokerURL    string
	MQTTClientID     string
	MQTTUsername     string
	MQTTPassword     string
	DatabaseURL      string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	TDengineURL      string
	TDengineUser     string
	TDenginePassword string
	JWTSecret        string
	RequestTimeout   int
}

func Default() Config {
	return Config{
		StorageMode:    "persistent",
		HTTPAddr:       ":8080",
		MQTTBrokerURL:  "tcp://localhost:1883",
		MQTTClientID:   "iot-platform",
		DatabaseURL:    "postgres://iot:iot@localhost:5432/iot?sslmode=disable",
		RedisAddr:      "localhost:6379",
		RedisDB:        0,
		TDengineURL:    "http://localhost:6041",
		JWTSecret:      "change-me-in-production",
		RequestTimeout: 10,
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
	if value := os.Getenv("IOT_JWT_SECRET"); value != "" {
		c.JWTSecret = value
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
	if len(c.JWTSecret) < 16 {
		return fmt.Errorf("JWT secret must be at least 16 characters")
	}
	if c.RequestTimeout <= 0 {
		return errors.New("request timeout must be greater than zero")
	}
	return nil
}
