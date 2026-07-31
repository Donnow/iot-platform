package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr       string
	MQTTBrokerURL  string
	DatabaseURL    string
	RedisAddr      string
	TDengineURL    string
	JWTSecret      string
	RequestTimeout int
}

func Default() Config {
	return Config{
		HTTPAddr:       ":8080",
		MQTTBrokerURL:  "tcp://localhost:1883",
		DatabaseURL:    "postgres://iot:iot@localhost:5432/iot?sslmode=disable",
		RedisAddr:      "localhost:6379",
		TDengineURL:    "http://localhost:6041",
		JWTSecret:      "change-me-in-production",
		RequestTimeout: 10,
	}
}

func FromEnv() Config {
	c := Default()
	if value := os.Getenv("IOT_HTTP_ADDR"); value != "" {
		c.HTTPAddr = value
	}
	if value := os.Getenv("IOT_MQTT_BROKER_URL"); value != "" {
		c.MQTTBrokerURL = value
	}
	if value := os.Getenv("IOT_DATABASE_URL"); value != "" {
		c.DatabaseURL = value
	}
	if value := os.Getenv("IOT_REDIS_ADDR"); value != "" {
		c.RedisAddr = value
	}
	if value := os.Getenv("IOT_TDENGINE_URL"); value != "" {
		c.TDengineURL = value
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
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return errors.New("HTTP address is required")
	}
	if strings.TrimSpace(c.MQTTBrokerURL) == "" {
		return errors.New("MQTT broker URL is required")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("database URL is required")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return errors.New("Redis address is required")
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
