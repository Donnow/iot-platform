package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"iot-perform/internal/platform/config"
	"iot-perform/internal/platform/httpapi"
	"iot-perform/internal/platform/memory"
	"iot-perform/internal/platform/mqtt"
	"iot-perform/internal/platform/observability"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.FromEnv()
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("platform stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	store := memory.New()
	metrics := observability.NewMetrics()
	mqttService, err := mqtt.NewServiceWithMetrics(mqtt.Config{
		BrokerURL: cfg.MQTTBrokerURL,
		ClientID:  cfg.MQTTClientID,
		Username:  cfg.MQTTUsername,
		Password:  cfg.MQTTPassword,
	}, store.Repositories(), logger, metrics)
	if err != nil {
		return err
	}
	server := httpapi.NewServerWithOptions(
		store.Repositories(),
		mqttService,
		httpapi.JWTAuthorizer{Secret: []byte(cfg.JWTSecret)},
		metrics,
		httpapi.InternalHooks{
			Authenticate: func(ctx context.Context, deviceID, secret string) (httpapi.InternalAuthResult, error) {
				result, err := mqttService.Authenticate(ctx, deviceID, secret)
				acl := make([]httpapi.InternalACLRule, 0, len(result.ACL))
				for _, rule := range result.ACL {
					acl = append(acl, httpapi.InternalACLRule{Topic: rule.Topic, Action: rule.Action})
				}
				return httpapi.InternalAuthResult{Allow: result.Allow, ACL: acl}, err
			},
			Lifecycle: mqttService.SetLifecycle,
		},
	)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.RequestTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.RequestTimeout) * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	go runMQTT(ctx, mqttService, logger)
	select {
	case <-ctx.Done():
	case err := <-serverErrors:
		_ = mqttService.Stop(context.Background())
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = mqttService.Stop(shutdownCtx)
	return httpServer.Shutdown(shutdownCtx)
}

func runMQTT(ctx context.Context, service *mqtt.Service, logger *slog.Logger) {
	backoff := time.Second
	for {
		if err := service.Start(ctx); err == nil {
			logger.Info("MQTT service started")
			return
		} else if ctx.Err() != nil {
			return
		} else {
			logger.Warn("MQTT service start failed; retrying", "error", err, "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}
