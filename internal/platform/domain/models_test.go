package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeviceSecretIsNotSerialized(t *testing.T) {
	data, err := json.Marshal(Device{DeviceID: "d1", DeviceSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"device_id":"d1"`) {
		t.Fatalf("device ID missing from serialized device: %s", data)
	}
	if strings.Contains(string(data), "secret") {
		t.Fatalf("device secret leaked in serialized device: %s", data)
	}
}
