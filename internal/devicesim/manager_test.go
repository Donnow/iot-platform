package devicesim

import "testing"

func TestManagerBuildsStressDeviceSet(t *testing.T) {
	config := testConfig(DeviceTypeTemperature)
	config.Count = 1000
	manager, err := NewManager(config, (&fakeFactory{}).Create, DeviceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	devices := manager.Devices()
	if len(devices) != 1000 {
		t.Fatalf("got %d devices, want 1000", len(devices))
	}
	seen := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		if _, exists := seen[device.DeviceID()]; exists {
			t.Fatalf("duplicate device ID %q", device.DeviceID())
		}
		seen[device.DeviceID()] = struct{}{}
	}
}
