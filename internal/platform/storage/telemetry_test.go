package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func TestTDengineEnsureSchema(t *testing.T) {
	var statements []string
	var mu sync.Mutex
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		statements = append(statements, string(body))
		mu.Unlock()
		return jsonResponse(`{"code":0,"rows":0}`), nil
	})}

	td := NewTDengine("http://tdengine.test", "root", "secret")
	td.httpClient = client
	if err := td.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statements) != 2 || !strings.HasPrefix(statements[0], "CREATE DATABASE") || !strings.HasPrefix(statements[1], "CREATE TABLE") {
		t.Fatalf("unexpected schema statements: %#v", statements)
	}
}

func TestTDengineTelemetryRepository(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	var statements []string
	var mu sync.Mutex
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		statements = append(statements, string(body))
		mu.Unlock()
		if strings.HasPrefix(string(body), "INSERT") {
			return jsonResponse(`{"code":0,"rows":1}`), nil
		}
		response := map[string]any{
			"code":        0,
			"column_meta": [][]any{{"ts", "TIMESTAMP", 8}, {"device_id", "BINARY", 8}, {"product_key", "BINARY", 8}, {"payload", "NCHAR", 8}},
			"data": [][]any{
				{float64(base.UnixMilli() + 10_000), "d-1", "sensor-v1", `{"temperature":20,"enabled":true}`},
				{float64(base.UnixMilli() + 20_000), "d-1", "sensor-v1", `{"temperature":22,"enabled":false}`},
			},
			"rows": 2,
		}
		encoded, _ := json.Marshal(response)
		return jsonResponse(string(encoded)), nil
	})}

	td := NewTDengine("http://tdengine.test", "", "")
	td.httpClient = client
	store := &Store{telemetry: td}
	ctx := context.Background()
	if err := store.AppendTelemetry(ctx, domain.Telemetry{
		DeviceID: "d'1", ProductKey: "sensor-v1", Timestamp: base, Values: map[string]any{"temperature": 21.5},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.QueryTelemetry(ctx, repository.TelemetryQuery{DeviceID: "d-1", Metric: "temperature", Interval: "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Values["temperature"] != 21.0 {
		t.Fatalf("unexpected aggregated telemetry: %#v", items)
	}
	snapshot, err := store.SnapshotTelemetry(ctx, "d-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["temperature"].Timestamp != base.Add(20*time.Second) || snapshot["enabled"].Values["enabled"] != false {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(statements[0], "d''1") || !strings.Contains(statements[0], "sensor-v1") {
		t.Fatalf("telemetry insert was not escaped as expected: %q", statements[0])
	}
	if !strings.Contains(statements[1], "device_id = 'd-1'") || !strings.Contains(statements[1], "ORDER BY ts ASC") {
		t.Fatalf("telemetry query was not constrained as expected: %q", statements[1])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
