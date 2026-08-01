package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/memory"
	"iot-perform/internal/platform/observability"
)

func TestHealthAndProductRoutes(t *testing.T) {
	store := memory.New()
	server := NewServer(store.Repositories(), nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/products", jsonBody(t, map[string]any{
		"name": "Temperature", "product_key": "temperature", "device_type": "sensor",
	}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create product status = %d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/products?page=1&page_size=10", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("list products status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "openapi: 3.0.3") {
		t.Fatalf("openapi status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/docs", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "SwaggerUIBundle") {
		t.Fatalf("swagger status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProductAndTelemetryValidation(t *testing.T) {
	store := memory.New()
	server := NewServer(store.Repositories(), nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/products", jsonBody(t, map[string]any{
		"name": "Temperature", "product_key": "temperature", "device_type": "sensor",
		"properties": []map[string]any{{"name": "temperature", "data_type": "float", "min_value": 0, "max_value": 50}},
	}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/products", jsonBody(t, map[string]any{
		"name": "Broken", "product_key": "broken", "properties": []map[string]any{{"name": "x", "data_type": "invalid"}},
	}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid property status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/devices", jsonBody(t, map[string]any{"device_id": "d1", "product_key": "temperature", "name": "D"}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create device status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/rules", jsonBody(t, map[string]any{"product_key": "temperature", "name": "bad", "property_name": "temperature", "operator": "contains", "action_type": "alarm"}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid rule status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/devices/d1/telemetry?interval=10s", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid interval status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceCommandAndShadowRoutes(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D", DeviceSecret: "secret"})
	server := NewServer(store.Repositories(), nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/devices/d1/commands", jsonBody(t, map[string]any{"method": "open"}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("command status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPut, "/api/devices/d1/shadow/desired", jsonBody(t, map[string]any{"targetTemp": 26}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("shadow status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestBearerAuthorizer(t *testing.T) {
	server := NewServer(memory.New().Repositories(), nil, BearerTokenAuthorizer{Token: "token"})
	request := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", response.Code)
	}
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized status=%d", response.Code)
	}
}

func TestEMQXInternalHooksAndMetrics(t *testing.T) {
	metrics := observability.NewMetrics()
	var lifecycle struct {
		deviceID string
		online   bool
		at       time.Time
	}
	server := NewServerWithOptions(memory.New().Repositories(), nil, nil, metrics, InternalHooks{
		Authenticate: func(_ context.Context, deviceID, secret string) (InternalAuthResult, error) {
			return InternalAuthResult{Allow: deviceID == "d1" && secret == "secret", ACL: []InternalACLRule{{Topic: "devices/pk/d1/telemetry", Action: "publish"}}}, nil
		},
		Lifecycle: func(_ context.Context, deviceID string, online bool, at time.Time) error {
			lifecycle.deviceID, lifecycle.online, lifecycle.at = deviceID, online, at
			return nil
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/internal/emqx/auth", jsonBody(t, map[string]any{"clientid": "d1", "password": "secret"}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"allow"`) {
		t.Fatalf("auth response=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/internal/emqx/webhook", jsonBody(t, map[string]any{"event": "client.connected", "clientid": "d1", "timestamp": int64(1722000000000)}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || lifecycle.deviceID != "d1" || !lifecycle.online || lifecycle.at.UnixMilli() != 1722000000000 {
		t.Fatalf("lifecycle response=%d state=%#v body=%s", response.Code, lifecycle, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "iot_platform_http_requests_total") {
		t.Fatalf("metrics response=%d body=%s", response.Code, response.Body.String())
	}
}

type otaPublisher struct {
	topics []string
}

func (p *otaPublisher) Publish(_ context.Context, topic string, _ byte, _ bool, _ []byte) error {
	p.topics = append(p.topics, topic)
	return nil
}

func TestFirmwareAndOTARoutes(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	_, _ = store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "P"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "online", ProductKey: "pk", Name: "Online"})
	_, _ = store.CreateDevice(ctx, domain.Device{DeviceID: "offline", ProductKey: "pk", Name: "Offline"})
	when := time.Unix(100, 0)
	if err := store.SetDeviceStatus(ctx, "online", domain.DeviceStatusOnline, &when); err != nil {
		t.Fatal(err)
	}
	publisher := &otaPublisher{}
	server := NewServer(store.Repositories(), publisher, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/firmwares", jsonBody(t, map[string]any{
		"product_key": "pk", "version": "1.2.3", "md5": "0123456789abcdef0123456789abcdef", "file_url": "https://example.test/fw.bin",
	}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("firmware status=%d body=%s", response.Code, response.Body.String())
	}
	firmwares, err := store.ListFirmwares(ctx, "pk")
	if err != nil || len(firmwares) != 1 {
		t.Fatalf("firmwares=%#v err=%v", firmwares, err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/ota/tasks", jsonBody(t, map[string]any{
		"product_key": "pk", "firmware_id": firmwares[0].ID, "target": "all",
	}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || len(publisher.topics) != 1 || publisher.topics[0] != "devices/pk/online/ota" {
		t.Fatalf("OTA status=%d topics=%#v body=%s", response.Code, publisher.topics, response.Body.String())
	}
	var task domain.OTATask
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.Summary[domain.OTAStagePending] != 2 {
		t.Fatalf("task summary=%#v", task.Summary)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/ota/tasks/"+task.ID, nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"summary"`) {
		t.Fatalf("task get status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/firmwares", jsonBody(t, map[string]any{
		"product_key": "pk", "version": "1.2.3", "md5": "0123456789abcdef0123456789abcdef", "file_url": "https://example.test/duplicate.bin",
	}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate firmware status=%d body=%s", response.Code, response.Body.String())
	}
}

func jsonBody(t *testing.T, value any) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(data))
}
