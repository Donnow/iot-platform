//go:build integration

package platform_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/httpapi"
	"iot-perform/internal/platform/memory"
	"iot-perform/internal/platform/mqtt"
)

type integrationPublisher struct {
	topics   []string
	payloads [][]byte
}

func (p *integrationPublisher) Publish(_ context.Context, topic string, _ byte, _ bool, payload []byte) error {
	p.topics = append(p.topics, topic)
	p.payloads = append(p.payloads, append([]byte(nil), payload...))
	return nil
}

func TestOTAHTTPMQTTFlow(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	if _, err := store.CreateProduct(ctx, domain.Product{ProductKey: "pk", Name: "Temperature"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDevice(ctx, domain.Device{DeviceID: "d1", ProductKey: "pk", Name: "D1"}); err != nil {
		t.Fatal(err)
	}
	publisher := &integrationPublisher{}
	server := httpapi.NewServer(store.Repositories(), publisher, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/firmwares", strings.NewReader(`{"product_key":"pk","version":"1.0.0","md5":"0123456789abcdef0123456789abcdef","file_url":"https://example.test/fw.bin"}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create firmware status=%d body=%s", response.Code, response.Body.String())
	}
	var firmware domain.Firmware
	if err := json.NewDecoder(response.Body).Decode(&firmware); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/ota/tasks", strings.NewReader(`{"product_key":"pk","firmware_id":"`+firmware.ID+`","target":"devices","target_device_ids":["d1"]}`))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create task status=%d body=%s", response.Code, response.Body.String())
	}
	var task domain.OTATask
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}

	service := mqtt.NewServiceWithClient(nil, store.Repositories(), nil)
	if err := service.ProcessMessage(ctx, "devices/pk/d1/event", []byte(`{"ts":1722000000000,"event_type":"ota_progress","data":{"version":"1.0.0","stage":"success","progress":100}}`)); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/ota/tasks/"+task.ID, nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":1`) {
		t.Fatalf("get task status=%d body=%s", response.Code, response.Body.String())
	}
	if len(publisher.topics) != 0 {
		t.Fatalf("offline device should not receive immediate OTA: %#v", publisher.topics)
	}
}
