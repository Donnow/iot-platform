package devicesim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigBuildsUniqueCredentials(t *testing.T) {
	config := DefaultConfig()
	config.Count = 3
	credentials, err := config.CredentialsForRun()
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 3 {
		t.Fatalf("got %d credentials, want 3", len(credentials))
	}
	if credentials[0].DeviceID == credentials[1].DeviceID {
		t.Fatal("generated device IDs must be unique")
	}
	if len(credentials[0].DeviceSecret) != 32 || !strings.HasPrefix(credentials[0].DeviceSecret, "sim-") {
		t.Fatalf("generated device secret = %q, want 32 characters", credentials[0].DeviceSecret)
	}
}

func TestConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing broker", mutate: func(c *Config) { c.BrokerURL = "" }},
		{name: "missing product", mutate: func(c *Config) { c.ProductKey = "" }},
		{name: "unknown type", mutate: func(c *Config) { c.DeviceType = "unknown" }},
		{name: "zero count", mutate: func(c *Config) { c.Count = 0 }},
		{name: "zero interval", mutate: func(c *Config) { c.Interval = 0 }},
		{name: "negative fluctuation", mutate: func(c *Config) { c.Fluctuation = -1 }},
		{name: "invalid threshold", mutate: func(c *Config) { c.SmokeThreshold = 101 }},
		{name: "wildcard product", mutate: func(c *Config) { c.ProductKey = "pk/+" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() returned nil")
			}
		})
	}
}

func TestLoadCredentialsJSONAndCSV(t *testing.T) {
	directory := t.TempDir()
	jsonPath := filepath.Join(directory, "devices.json")
	if err := os.WriteFile(jsonPath, []byte(`[{"device_id":"d1","device_secret":"s1"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	jsonCredentials, err := LoadCredentials(jsonPath)
	if err != nil || len(jsonCredentials) != 1 || jsonCredentials[0].DeviceID != "d1" {
		t.Fatalf("JSON credentials = %#v, err = %v", jsonCredentials, err)
	}

	csvPath := filepath.Join(directory, "devices.csv")
	if err := os.WriteFile(csvPath, []byte("device_id,device_secret\nd2,s2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	csvCredentials, err := LoadCredentials(csvPath)
	if err != nil || len(csvCredentials) != 1 || csvCredentials[0].DeviceID != "d2" {
		t.Fatalf("CSV credentials = %#v, err = %v", csvCredentials, err)
	}
}

func TestConfigRejectsDuplicateCredentials(t *testing.T) {
	config := DefaultConfig()
	config.Count = 2
	config.Credentials = []DeviceCredential{
		{DeviceID: "same", DeviceSecret: "s1"},
		{DeviceID: "same", DeviceSecret: "s2"},
	}
	if _, err := config.CredentialsForRun(); err == nil {
		t.Fatal("duplicate credentials should be rejected")
	}
}
