package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Metrics struct {
	httpRequests          atomic.Uint64
	mqttMessages          atomic.Uint64
	mqttErrors            atomic.Uint64
	mqttDropped           atomic.Uint64
	telemetryBatchFlushed atomic.Uint64
	telemetryBatchRetries atomic.Uint64
	telemetryBatchFailed  atomic.Uint64
	ruleMatches           atomic.Uint64
	alarmsCreated         atomic.Uint64
}

func NewMetrics() *Metrics { return &Metrics{} }

func (m *Metrics) IncHTTPRequests() { m.httpRequests.Add(1) }
func (m *Metrics) IncMQTTMessages() { m.mqttMessages.Add(1) }
func (m *Metrics) IncMQTTErrors()   { m.mqttErrors.Add(1) }
func (m *Metrics) IncMQTTDropped()  { m.mqttDropped.Add(1) }
func (m *Metrics) IncTelemetryBatchFlushed(rows int) {
	if rows > 0 {
		m.telemetryBatchFlushed.Add(uint64(rows))
	}
}
func (m *Metrics) IncTelemetryBatchRetries()  { m.telemetryBatchRetries.Add(1) }
func (m *Metrics) IncTelemetryBatchFailures() { m.telemetryBatchFailed.Add(1) }
func (m *Metrics) IncRuleMatches()            { m.ruleMatches.Add(1) }
func (m *Metrics) IncAlarmsCreated()          { m.alarmsCreated.Add(1) }

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprintf(writer,
			"# HELP iot_platform_http_requests_total Total HTTP requests handled.\n"+
				"# TYPE iot_platform_http_requests_total counter\n"+
				"iot_platform_http_requests_total %d\n"+
				"# HELP iot_platform_mqtt_messages_total Total MQTT messages routed into the consumer pipeline.\n"+
				"# TYPE iot_platform_mqtt_messages_total counter\n"+
				"iot_platform_mqtt_messages_total %d\n"+
				"# HELP iot_platform_mqtt_errors_total Total MQTT processing errors.\n"+
				"# TYPE iot_platform_mqtt_errors_total counter\n"+
				"iot_platform_mqtt_errors_total %d\n"+
				"# HELP iot_platform_mqtt_dropped_total Total MQTT messages dropped because the consumer queue was full.\n"+
				"# TYPE iot_platform_mqtt_dropped_total counter\n"+
				"iot_platform_mqtt_dropped_total %d\n"+
				"# HELP iot_platform_telemetry_batch_flushed_total Total telemetry rows flushed to TDengine in batches.\n"+
				"# TYPE iot_platform_telemetry_batch_flushed_total counter\n"+
				"iot_platform_telemetry_batch_flushed_total %d\n"+
				"# HELP iot_platform_telemetry_batch_retries_total Total telemetry batch flush retries.\n"+
				"# TYPE iot_platform_telemetry_batch_retries_total counter\n"+
				"iot_platform_telemetry_batch_retries_total %d\n"+
				"# HELP iot_platform_telemetry_batch_failures_total Total telemetry batches quarantined after retries.\n"+
				"# TYPE iot_platform_telemetry_batch_failures_total counter\n"+
				"iot_platform_telemetry_batch_failures_total %d\n"+
				"# HELP iot_platform_rule_matches_total Total telemetry samples matching rules.\n"+
				"# TYPE iot_platform_rule_matches_total counter\n"+
				"iot_platform_rule_matches_total %d\n"+
				"# HELP iot_platform_alarms_created_total Total alarms created by rules.\n"+
				"# TYPE iot_platform_alarms_created_total counter\n"+
				"iot_platform_alarms_created_total %d\n",
			m.httpRequests.Load(), m.mqttMessages.Load(), m.mqttErrors.Load(), m.mqttDropped.Load(),
			m.telemetryBatchFlushed.Load(), m.telemetryBatchRetries.Load(), m.telemetryBatchFailed.Load(),
			m.ruleMatches.Load(), m.alarmsCreated.Load())
	})
}
