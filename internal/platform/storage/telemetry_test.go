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
		if strings.HasPrefix(string(body), "DESCRIBE") {
			return jsonResponse(`{"code":0,"column_meta":[["Field","VARCHAR",64],["Type","VARCHAR",64],["Length","INT",4],["Note","VARCHAR",64]],"data":[["ts","TIMESTAMP",8,""],["payload","NCHAR",4096,""],["device_id","BINARY",128,"TAG"],["product_key","BINARY",128,"TAG"]],"rows":4}`), nil
		}
		return jsonResponse(`{"code":0,"rows":0}`), nil
	})}

	td := NewTDengine("http://tdengine.test", "root", "secret")
	td.httpClient = client
	if err := td.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statements) != 3 {
		t.Fatalf("expected 3 schema statements, got %d: %#v", len(statements), statements)
	}
	if !strings.HasPrefix(statements[0], "CREATE DATABASE") {
		t.Fatalf("first statement should create the database: %q", statements[0])
	}
	if !strings.HasPrefix(statements[1], "CREATE STABLE") || !strings.Contains(statements[1], "TAGS") {
		t.Fatalf("second statement should create the stable with tags: %q", statements[1])
	}
	if !strings.HasPrefix(statements[2], "DESCRIBE") {
		t.Fatalf("third statement should describe the stable: %q", statements[2])
	}
}

func TestTDengineEnsureSchemaRejectsWrongSchema(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if strings.HasPrefix(string(body), "DESCRIBE") {
			return jsonResponse(`{"code":0,"data":[["ts","TIMESTAMP",8],["device_id","BINARY",128],["product_key","BINARY",128],["payload","NCHAR",4096]],"rows":4}`), nil
		}
		return jsonResponse(`{"code":0,"rows":0}`), nil
	})}

	td := NewTDengine("http://tdengine.test", "", "")
	td.httpClient = client
	if err := td.EnsureSchema(context.Background()); err == nil {
		t.Fatal("expected EnsureSchema to reject a plain table schema without tags")
	}
}

func TestTelemetryChildTable(t *testing.T) {
	table := telemetryChildTable("d-1")
	if table != "t_1987a88fc39f6b7f" {
		t.Fatalf(`telemetryChildTable("d-1") = %q, want "t_1987a88fc39f6b7f"`, table)
	}
	if len(table) != 18 {
		t.Fatalf("child table name should be fixed length 18, got %d: %q", len(table), table)
	}
	if telemetryChildTable("d-1") != telemetryChildTable("d-1") {
		t.Fatal("child table name must be deterministic for a device")
	}
	if telemetryChildTable("d-1") == telemetryChildTable("d-2") {
		t.Fatal("distinct devices must map to distinct child tables")
	}
	for _, r := range telemetryChildTable("my-device@01") {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			t.Fatalf("child table name contains invalid character %q", r)
		}
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
		statement := string(body)
		if strings.HasPrefix(statement, "INSERT INTO") {
			return jsonResponse(`{"code":0,"rows":1}`), nil
		}
		if strings.HasPrefix(statement, "SELECT product_key") {
			return jsonResponse(`{"code":0,"column_meta":[["product_key","BINARY",8]],"data":[["sensor-v1"]],"rows":1}`), nil
		}
		if strings.HasPrefix(statement, "SELECT ts, payload") {
			response := map[string]any{
				"code":        0,
				"column_meta": [][]any{{"ts", "TIMESTAMP", 8}, {"payload", "NCHAR", 8}},
				"data": [][]any{
					{float64(base.UnixMilli() + 10_000), `{"temperature":20,"enabled":true}`},
					{float64(base.UnixMilli() + 20_000), `{"temperature":22,"enabled":false}`},
				},
				"rows": 2,
			}
			encoded, _ := json.Marshal(response)
			return jsonResponse(string(encoded)), nil
		}
		return jsonResponse(`{"code":0,"rows":0}`), nil
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
	if items[0].DeviceID != "d-1" || items[0].ProductKey != "sensor-v1" {
		t.Fatalf("telemetry device/product not filled from code: %#v", items[0])
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
	if !strings.Contains(statements[0], "INSERT INTO t_") || !strings.Contains(statements[0], "USING "+telemetryStable) || !strings.Contains(statements[0], "TAGS ('d''1', 'sensor-v1')") {
		t.Fatalf("telemetry insert did not target a child table as expected: %q", statements[0])
	}
	if !strings.Contains(statements[1], "SELECT product_key FROM t_1987a88fc39f6b7f") {
		t.Fatalf("product_key tag was not resolved from the child table: %q", statements[1])
	}
	if !strings.Contains(statements[2], "SELECT ts, payload FROM t_1987a88fc39f6b7f") || !strings.Contains(statements[2], "ORDER BY ts ASC") {
		t.Fatalf("telemetry query was not constrained as expected: %q", statements[2])
	}
}

func TestTDengineAppendTelemetryBatch(t *testing.T) {
	var statements []string
	var mu sync.Mutex
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		statements = append(statements, string(body))
		mu.Unlock()
		return jsonResponse(`{"code":0,"rows":3}`), nil
	})}

	td := NewTDengine("http://tdengine.test", "", "")
	td.httpClient = client
	store := &Store{telemetry: td}
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := store.AppendTelemetryBatch(context.Background(), []domain.Telemetry{
		{DeviceID: "d-1", ProductKey: "sensor-v1", Timestamp: base, Values: map[string]any{"temperature": 20.0}},
		{DeviceID: "d-1", ProductKey: "sensor-v1", Timestamp: base.Add(time.Second), Values: map[string]any{"temperature": 21.0}},
		{DeviceID: "d-2", ProductKey: "sensor-v1", Timestamp: base, Values: map[string]any{"humidity": 50.0}},
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statements) != 1 {
		t.Fatalf("expected 1 batch statement, got %d: %#v", len(statements), statements)
	}
	stmt := statements[0]
	if !strings.HasPrefix(stmt, "INSERT INTO ") {
		t.Fatalf("batch should be a single multi-table insert: %q", stmt)
	}
	if !strings.Contains(stmt, "USING "+telemetryStable+" TAGS ('d-1', 'sensor-v1') VALUES (") {
		t.Fatalf("missing d-1 USING group: %q", stmt)
	}
	if !strings.Contains(stmt, ") (") {
		t.Fatalf("expected multiple values per device group: %q", stmt)
	}
	if !strings.Contains(stmt, "USING "+telemetryStable+" TAGS ('d-2', 'sensor-v1') VALUES (") {
		t.Fatalf("missing d-2 USING group: %q", stmt)
	}
}

func TestTDengineAppendTelemetryBatchEmpty(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("should not query TDengine for an empty batch")
		return nil, nil
	})}
	td := NewTDengine("http://tdengine.test", "", "")
	td.httpClient = client
	store := &Store{telemetry: td}
	if err := store.AppendTelemetryBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestTDengineQueryMissingChildReturnsEmpty(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if strings.HasPrefix(string(body), "SELECT product_key") {
			return jsonResponse(`{"code":9585,"desc":"Table does not exist: t_deadbeef","rows":0}`), nil
		}
		return jsonResponse(`{"code":0,"rows":0}`), nil
	})}

	td := NewTDengine("http://tdengine.test", "", "")
	td.httpClient = client
	store := &Store{telemetry: td}
	items, err := store.QueryTelemetry(context.Background(), repository.TelemetryQuery{DeviceID: "no-such-device"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty results for missing child table, got %#v", items)
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
