package devicesim

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DeviceType string

const (
	DeviceTypeTemperature    DeviceType = "temperature"
	DeviceTypeSmoke          DeviceType = "smoke"
	DeviceTypeDoor           DeviceType = "door"
	DeviceTypeAirConditioner DeviceType = "air-conditioner"
)

type DeviceCredential struct {
	DeviceID     string `json:"device_id"`
	DeviceSecret string `json:"device_secret"`
}

type Config struct {
	BrokerURL      string
	ProductKey     string
	DeviceType     DeviceType
	Count          int
	Interval       time.Duration
	Fluctuation    float64
	SmokeThreshold float64
	DevicePrefix   string
	Seed           int64
	Credentials    []DeviceCredential
	Stress         bool
}

func DefaultConfig() Config {
	return Config{
		BrokerURL:      "tcp://localhost:1883",
		ProductKey:     "demo-product",
		DeviceType:     DeviceTypeTemperature,
		Count:          1,
		Interval:       5 * time.Second,
		Fluctuation:    1,
		SmokeThreshold: 50,
		DevicePrefix:   "sim",
		Seed:           1,
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.BrokerURL) == "" {
		return errors.New("broker URL is required")
	}
	u, err := url.Parse(c.BrokerURL)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("invalid broker URL %q", c.BrokerURL)
	}
	if strings.TrimSpace(c.ProductKey) == "" || strings.ContainsAny(c.ProductKey, "/+#") {
		return fmt.Errorf("invalid product key %q", c.ProductKey)
	}
	if !validDeviceType(c.DeviceType) {
		return fmt.Errorf("unsupported device type %q", c.DeviceType)
	}
	if c.Count < 1 {
		return errors.New("count must be at least 1")
	}
	if c.Interval <= 0 {
		return errors.New("interval must be greater than zero")
	}
	if c.Fluctuation < 0 {
		return errors.New("fluctuation cannot be negative")
	}
	if c.SmokeThreshold < 0 || c.SmokeThreshold > 100 {
		return errors.New("smoke threshold must be between 0 and 100")
	}
	if strings.TrimSpace(c.DevicePrefix) == "" || strings.ContainsAny(c.DevicePrefix, "/+#") {
		return fmt.Errorf("invalid device prefix %q", c.DevicePrefix)
	}
	for i, credential := range c.Credentials {
		if err := credential.Validate(); err != nil {
			return fmt.Errorf("credential %d: %w", i, err)
		}
	}
	if len(c.Credentials) > 0 && len(c.Credentials) < c.Count {
		return fmt.Errorf("count %d exceeds credential count %d", c.Count, len(c.Credentials))
	}
	return nil
}

func (c DeviceCredential) Validate() error {
	if strings.TrimSpace(c.DeviceID) == "" || strings.ContainsAny(c.DeviceID, "/+#") {
		return errors.New("device ID is required and cannot contain MQTT wildcards")
	}
	if strings.TrimSpace(c.DeviceSecret) == "" {
		return errors.New("device secret is required")
	}
	return nil
}

func (c Config) CredentialsForRun() ([]DeviceCredential, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if len(c.Credentials) > 0 {
		credentials := append([]DeviceCredential(nil), c.Credentials[:c.Count]...)
		if err := ensureUniqueIDs(credentials); err != nil {
			return nil, err
		}
		return credentials, nil
	}

	credentials := make([]DeviceCredential, c.Count)
	for i := range credentials {
		credentials[i] = DeviceCredential{
			DeviceID:     fmt.Sprintf("%s-%s-%03d", c.DevicePrefix, c.DeviceType, i+1),
			DeviceSecret: fmt.Sprintf("sim-%028d", i+1),
		}
	}
	return credentials, nil
}

func ensureUniqueIDs(credentials []DeviceCredential) error {
	seen := make(map[string]struct{}, len(credentials))
	for _, credential := range credentials {
		if _, exists := seen[credential.DeviceID]; exists {
			return fmt.Errorf("duplicate device ID %q", credential.DeviceID)
		}
		seen[credential.DeviceID] = struct{}{}
	}
	return nil
}

func LoadCredentials(path string) ([]DeviceCredential, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return parseJSONCredentials(data)
	case ".csv":
		return parseCSVCredentials(strings.NewReader(string(data)))
	default:
		return nil, fmt.Errorf("unsupported credential file extension %q", ext)
	}
}

func parseJSONCredentials(data []byte) ([]DeviceCredential, error) {
	var credentials []DeviceCredential
	if err := json.Unmarshal(data, &credentials); err == nil {
		return validateCredentials(credentials)
	}
	var wrapper struct {
		Devices []DeviceCredential `json:"devices"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse credentials JSON: %w", err)
	}
	return validateCredentials(wrapper.Devices)
}

func parseCSVCredentials(reader io.Reader) ([]DeviceCredential, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read credentials CSV header: %w", err)
	}
	indices := make(map[string]int, len(header))
	for i, name := range header {
		indices[strings.ToLower(strings.TrimSpace(name))] = i
	}
	idIndex, okID := indices["device_id"]
	secretIndex, okSecret := indices["device_secret"]
	if !okID || !okSecret {
		return nil, errors.New("credentials CSV must contain device_id and device_secret columns")
	}
	var credentials []DeviceCredential
	for row := 2; ; row++ {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read credentials CSV row %d: %w", row, err)
		}
		if len(record) <= idIndex || len(record) <= secretIndex {
			return nil, fmt.Errorf("credentials CSV row %d is missing fields", row)
		}
		credentials = append(credentials, DeviceCredential{
			DeviceID:     strings.TrimSpace(record[idIndex]),
			DeviceSecret: strings.TrimSpace(record[secretIndex]),
		})
	}
	return validateCredentials(credentials)
}

func validateCredentials(credentials []DeviceCredential) ([]DeviceCredential, error) {
	if len(credentials) == 0 {
		return nil, errors.New("credential file contains no devices")
	}
	if err := ensureUniqueIDs(credentials); err != nil {
		return nil, err
	}
	for i, credential := range credentials {
		if err := credential.Validate(); err != nil {
			return nil, fmt.Errorf("credential %d: %w", i, err)
		}
	}
	return credentials, nil
}

func validDeviceType(deviceType DeviceType) bool {
	switch deviceType {
	case DeviceTypeTemperature, DeviceTypeSmoke, DeviceTypeDoor, DeviceTypeAirConditioner:
		return true
	default:
		return false
	}
}
