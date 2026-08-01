package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

const (
	telemetryDatabase = "iot_telemetry"
	telemetryTable    = telemetryDatabase + ".telemetry"
	maxTelemetryBody  = 16 << 20
)

// TDengine is the small REST client used by the platform's telemetry repository.
// Keeping the client here avoids coupling the rest of the platform to TDengine's
// response representation while preserving the dynamic property payload.
type TDengine struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

func NewTDengine(baseURL, username, password string) *TDengine {
	return &TDengine{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type tdengineResponse struct {
	Code       int     `json:"code"`
	Desc       string  `json:"desc"`
	ColumnMeta [][]any `json:"column_meta"`
	Data       [][]any `json:"data"`
	Rows       int     `json:"rows"`
}

func (t *TDengine) query(ctx context.Context, statement string) (tdengineResponse, error) {
	if t == nil || strings.TrimSpace(t.baseURL) == "" {
		return tdengineResponse{}, errors.New("TDengine client is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/rest/sql", strings.NewReader(statement))
	if err != nil {
		return tdengineResponse{}, err
	}
	req.Header.Set("Content-Type", "text/plain")
	if t.username != "" {
		req.SetBasicAuth(t.username, t.password)
	}
	response, err := t.httpClient.Do(req)
	if err != nil {
		return tdengineResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxTelemetryBody))
	if err != nil {
		return tdengineResponse{}, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return tdengineResponse{}, fmt.Errorf("TDengine returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result tdengineResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return tdengineResponse{}, fmt.Errorf("decode TDengine response: %w", err)
	}
	if result.Code != 0 {
		return tdengineResponse{}, fmt.Errorf("TDengine query failed (%d): %s", result.Code, result.Desc)
	}
	return result, nil
}

func (t *TDengine) EnsureSchema(ctx context.Context) error {
	statements := []string{
		"CREATE DATABASE IF NOT EXISTS " + telemetryDatabase + " KEEP 3650",
		"CREATE TABLE IF NOT EXISTS " + telemetryTable + " (ts TIMESTAMP, device_id BINARY(128), product_key BINARY(128), payload NCHAR(4096))",
	}
	for _, statement := range statements {
		if _, err := t.query(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AppendTelemetry(ctx context.Context, sample domain.Telemetry) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if sample.DeviceID == "" || sample.Timestamp.IsZero() || len(sample.Values) == 0 {
		return errors.New("telemetry requires device, timestamp, and values")
	}
	if s.telemetry == nil {
		return errors.New("telemetry storage is not configured")
	}
	payload, err := json.Marshal(sample.Values)
	if err != nil {
		return fmt.Errorf("encode telemetry: %w", err)
	}
	statement := fmt.Sprintf("INSERT INTO %s (ts, device_id, product_key, payload) VALUES (%d, '%s', '%s', '%s')",
		telemetryTable,
		sample.Timestamp.UnixMilli(),
		escapeSQL(sample.DeviceID),
		escapeSQL(sample.ProductKey),
		escapeSQL(string(payload)),
	)
	_, err = s.telemetry.query(ctx, statement)
	return err
}

func (s *Store) QueryTelemetry(ctx context.Context, query repository.TelemetryQuery) ([]domain.Telemetry, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if query.DeviceID == "" {
		return nil, errors.New("telemetry query requires device")
	}
	if s.telemetry == nil {
		return nil, errors.New("telemetry storage is not configured")
	}
	where := []string{"device_id = '" + escapeSQL(query.DeviceID) + "'"}
	if !query.From.IsZero() {
		where = append(where, "ts >= "+strconv.FormatInt(query.From.UnixMilli(), 10))
	}
	if !query.To.IsZero() {
		where = append(where, "ts <= "+strconv.FormatInt(query.To.UnixMilli(), 10))
	}
	statement := "SELECT ts, device_id, product_key, payload FROM " + telemetryTable + " WHERE " + strings.Join(where, " AND ") + " ORDER BY ts ASC"
	if query.Interval == "" || query.Interval == "raw" {
		if query.Limit > 0 {
			statement += " LIMIT " + strconv.Itoa(query.Limit)
		}
	}
	response, err := s.telemetry.query(ctx, statement)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Telemetry, 0, len(response.Data))
	for _, row := range response.Data {
		sample, err := decodeTelemetryRow(response.ColumnMeta, row)
		if err != nil {
			return nil, err
		}
		if query.Metric != "" {
			value, ok := sample.Values[query.Metric]
			if !ok {
				continue
			}
			sample.Values = map[string]any{query.Metric: value}
		}
		result = append(result, sample)
	}
	if query.Interval != "" && query.Interval != "raw" {
		result = aggregateTelemetry(result, query.Interval)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	if query.Limit > 0 && len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result, nil
}

func (s *Store) SnapshotTelemetry(ctx context.Context, deviceID string) (map[string]domain.Telemetry, error) {
	samples, err := s.QueryTelemetry(ctx, repository.TelemetryQuery{DeviceID: deviceID})
	if err != nil {
		return nil, err
	}
	snapshot := make(map[string]domain.Telemetry)
	for _, sample := range samples {
		for metric := range sample.Values {
			previous, exists := snapshot[metric]
			if !exists || previous.Timestamp.Before(sample.Timestamp) {
				snapshot[metric] = cloneTelemetry(sample)
			}
		}
	}
	return snapshot, nil
}

func decodeTelemetryRow(columnMeta [][]any, row []any) (domain.Telemetry, error) {
	values := make(map[string]any)
	var sample domain.Telemetry
	for index, column := range columnMeta {
		if index >= len(row) || len(column) == 0 {
			continue
		}
		name, ok := column[0].(string)
		if !ok {
			continue
		}
		switch name {
		case "ts":
			timestamp, err := parseTDengineTime(row[index])
			if err != nil {
				return domain.Telemetry{}, err
			}
			sample.Timestamp = timestamp
		case "device_id":
			sample.DeviceID = fmt.Sprint(row[index])
		case "product_key":
			sample.ProductKey = fmt.Sprint(row[index])
		case "payload":
			if err := scanJSON(row[index], &values); err != nil {
				return domain.Telemetry{}, fmt.Errorf("decode telemetry payload: %w", err)
			}
		}
	}
	sample.Values = values
	return sample, nil
}

func parseTDengineTime(value any) (time.Time, error) {
	switch value := value.(type) {
	case float64:
		return time.UnixMilli(int64(value)).UTC(), nil
	case json.Number:
		milliseconds, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.UnixMilli(milliseconds).UTC(), nil
	case string:
		if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
			return time.UnixMilli(milliseconds).UTC(), nil
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05.999999999"} {
			if timestamp, err := time.Parse(layout, value); err == nil {
				return timestamp.UTC(), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("unsupported TDengine timestamp %v", value)
}

func aggregateTelemetry(samples []domain.Telemetry, interval string) []domain.Telemetry {
	duration, ok := map[string]time.Duration{"1m": time.Minute, "5m": 5 * time.Minute, "1h": time.Hour}[interval]
	if !ok || len(samples) == 0 {
		return samples
	}
	type metricValue struct {
		sum   float64
		count int
		last  any
	}
	type bucket struct {
		deviceID   string
		productKey string
		values     map[string]metricValue
	}
	buckets := make(map[time.Time]*bucket)
	for _, sample := range samples {
		at := sample.Timestamp.UTC().Truncate(duration)
		group := buckets[at]
		if group == nil {
			group = &bucket{deviceID: sample.DeviceID, productKey: sample.ProductKey, values: make(map[string]metricValue)}
			buckets[at] = group
		}
		for name, raw := range sample.Values {
			entry := group.values[name]
			entry.last = raw
			if value, ok := numericTelemetryValue(raw); ok {
				entry.sum += value
				entry.count++
			}
			group.values[name] = entry
		}
	}
	result := make([]domain.Telemetry, 0, len(buckets))
	for at, group := range buckets {
		values := make(map[string]any, len(group.values))
		for name, entry := range group.values {
			if entry.count > 0 {
				values[name] = entry.sum / float64(entry.count)
			} else {
				values[name] = entry.last
			}
		}
		result = append(result, domain.Telemetry{DeviceID: group.deviceID, ProductKey: group.productKey, Timestamp: at, Values: values})
	}
	return result
}

func numericTelemetryValue(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func escapeSQL(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "'", "''")
}

func cloneTelemetry(sample domain.Telemetry) domain.Telemetry {
	copy := sample
	copy.Values = make(map[string]any, len(sample.Values))
	for key, value := range sample.Values {
		copy.Values[key] = value
	}
	return copy
}

var _ repository.TelemetryRepository = (*Store)(nil)
