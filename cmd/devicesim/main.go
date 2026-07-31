package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"iot-perform/internal/devicesim"
)

func main() {
	config := devicesim.DefaultConfig()
	var interval string
	var credentialPath string
	var deviceType string
	flag.StringVar(&config.BrokerURL, "broker", config.BrokerURL, "MQTT broker URL")
	flag.StringVar(&config.ProductKey, "product-key", config.ProductKey, "product key")
	flag.StringVar(&deviceType, "type", string(config.DeviceType), "device type: temperature, smoke, door, air-conditioner")
	flag.IntVar(&config.Count, "count", config.Count, "number of devices")
	flag.StringVar(&interval, "interval", config.Interval.String(), "telemetry interval, for example 5s")
	flag.Float64Var(&config.Fluctuation, "fluctuation", config.Fluctuation, "maximum single-tick sensor fluctuation")
	flag.Float64Var(&config.SmokeThreshold, "smoke-threshold", config.SmokeThreshold, "smoke alarm threshold")
	flag.StringVar(&config.DevicePrefix, "device-prefix", config.DevicePrefix, "prefix for generated device IDs")
	flag.Int64Var(&config.Seed, "seed", config.Seed, "random seed")
	flag.StringVar(&credentialPath, "credentials", "", "CSV or JSON device credential file")
	flag.BoolVar(&config.Stress, "stress", false, "enable stress mode logging")
	flag.Parse()

	config.DeviceType = devicesim.DeviceType(strings.ToLower(strings.TrimSpace(deviceType)))
	parsedInterval, err := time.ParseDuration(interval)
	if err != nil {
		fail("parse interval", err)
	}
	config.Interval = parsedInterval
	if credentialPath != "" {
		config.Credentials, err = devicesim.LoadCredentials(credentialPath)
		if err != nil {
			fail("load credentials", err)
		}
	}
	if err := config.Validate(); err != nil {
		fail("validate configuration", err)
	}
	if config.Stress {
		slog.Info("stress mode enabled", "count", config.Count, "interval", config.Interval)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager, err := devicesim.NewManager(config, devicesim.NewPahoFactory(), devicesim.DefaultDeviceOptions())
	if err != nil {
		fail("create device manager", err)
	}
	slog.Info("starting device simulator", "type", config.DeviceType, "count", len(manager.Devices()), "broker", config.BrokerURL)
	if err := manager.Run(ctx); err != nil {
		fail("run device simulator", err)
	}
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "devicesim: %s: %v\n", action, err)
	os.Exit(2)
}
