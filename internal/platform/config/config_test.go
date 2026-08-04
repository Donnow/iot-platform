package config

import "testing"

func TestDefaultConfigIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRejectsShortJWTSecret(t *testing.T) {
	c := Default()
	c.JWTSecret = "short"
	if err := c.Validate(); err == nil {
		t.Fatal("short JWT secret should be rejected")
	}
}

func TestConfigRejectsUnknownStorageMode(t *testing.T) {
	c := Default()
	c.StorageMode = "remote"
	if err := c.Validate(); err == nil {
		t.Fatal("unknown storage mode should be rejected")
	}
}

func TestConfigReadsConsumerPipelineEnv(t *testing.T) {
	t.Setenv("IOT_MQTT_WORKERS", "8")
	t.Setenv("IOT_MQTT_QUEUE_SIZE", "512")
	t.Setenv("IOT_TELEMETRY_BATCH_SIZE", "500")
	t.Setenv("IOT_TELEMETRY_BATCH_INTERVAL_MS", "150")
	t.Setenv("IOT_PRODUCT_CACHE_TTL_SECONDS", "30")
	c := FromEnv()
	if c.MQTTWorkers != 8 || c.MQTTQueueSize != 512 || c.TelemetryBatchSize != 500 || c.TelemetryBatchIntervalMS != 150 || c.ProductCacheTTLSeconds != 30 {
		t.Fatalf("unexpected consumer pipeline config: %+v", c)
	}
}

func TestConfigRejectsNonPositiveConsumerPipeline(t *testing.T) {
	c := Default()
	c.MQTTWorkers = 0
	if err := c.Validate(); err == nil {
		t.Fatal("zero workers should be rejected")
	}
	c = Default()
	c.TelemetryBatchIntervalMS = 0
	if err := c.Validate(); err == nil {
		t.Fatal("zero batch interval should be rejected")
	}
}
