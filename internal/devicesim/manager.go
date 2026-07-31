package devicesim

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

type Manager struct {
	devices []*Device
}

func NewManager(config Config, factory ClientFactory, options DeviceOptions) (*Manager, error) {
	if factory == nil {
		return nil, errors.New("MQTT client factory is required")
	}
	credentials, err := config.CredentialsForRun()
	if err != nil {
		return nil, err
	}
	devices := make([]*Device, 0, len(credentials))
	for index, credential := range credentials {
		device, err := NewDevice(config, credential, index, factory, options)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return &Manager{devices: devices}, nil
}

func (m *Manager) Devices() []*Device {
	return append([]*Device(nil), m.devices...)
}

func (m *Manager) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(m.devices))
	for _, device := range m.devices {
		device := device
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := device.Run(ctx); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		return err
	}
	return nil
}

func DefaultDeviceOptions() DeviceOptions {
	return DeviceOptions{Logger: slog.Default()}
}
