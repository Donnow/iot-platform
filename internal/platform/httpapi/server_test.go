package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/memory"
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

func jsonBody(t *testing.T, value any) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(data))
}
