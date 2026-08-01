package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/messaging"
	"iot-perform/internal/platform/observability"
	"iot-perform/internal/platform/repository"
)

type Authorizer interface {
	Authorize(*http.Request) error
}

type AllowAllAuthorizer struct{}

func (AllowAllAuthorizer) Authorize(*http.Request) error { return nil }

type BearerTokenAuthorizer struct {
	Token string
}

func (a BearerTokenAuthorizer) Authorize(request *http.Request) error {
	if a.Token == "" {
		return nil
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(request.Header.Get("Authorization"), prefix) || strings.TrimPrefix(request.Header.Get("Authorization"), prefix) != a.Token {
		return errors.New("unauthorized")
	}
	return nil
}

type InternalACLRule struct {
	Topic  string `json:"topic"`
	Action string `json:"action"`
}

type InternalAuthResult struct {
	Allow bool
	ACL   []InternalACLRule
}

type InternalHooks struct {
	Authenticate func(context.Context, string, string) (InternalAuthResult, error)
	Lifecycle    func(context.Context, string, bool, time.Time) error
}

type Server struct {
	repos      repository.Repositories
	publisher  messaging.Publisher
	authorizer Authorizer
	metrics    *observability.Metrics
	hooks      InternalHooks
}

func NewServer(repos repository.Repositories, publisher messaging.Publisher, authorizer Authorizer) *Server {
	return NewServerWithOptions(repos, publisher, authorizer, nil, InternalHooks{})
}

func NewServerWithOptions(repos repository.Repositories, publisher messaging.Publisher, authorizer Authorizer, metrics *observability.Metrics, hooks InternalHooks) *Server {
	if authorizer == nil {
		authorizer = AllowAllAuthorizer{}
	}
	return &Server{repos: repos, publisher: publisher, authorizer: authorizer, metrics: metrics, hooks: hooks}
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if s.metrics != nil {
		s.metrics.IncHTTPRequests()
	}
	if request.URL.Path == "/healthz" {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if request.URL.Path == "/metrics" {
		if s.metrics != nil {
			s.metrics.Handler().ServeHTTP(writer, request)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"backend_http_requests_total": 1})
		return
	}
	if strings.HasPrefix(request.URL.Path, "/internal/emqx/") {
		s.handleEMQX(writer, request)
		return
	}
	if err := s.authorizer.Authorize(request); err != nil {
		writeError(writer, http.StatusUnauthorized, err)
		return
	}
	segments := pathSegments(request.URL.Path)
	if len(segments) < 2 || segments[0] != "api" {
		writeError(writer, http.StatusNotFound, errors.New("route not found"))
		return
	}
	switch segments[1] {
	case "products":
		s.handleProducts(writer, request, segments[2:])
	case "devices":
		s.handleDevices(writer, request, segments[2:])
	case "alarms":
		s.handleAlarms(writer, request, segments[2:])
	case "rules":
		s.handleRules(writer, request, segments[2:])
	default:
		writeError(writer, http.StatusNotFound, errors.New("route not found"))
	}
}

func (s *Server) handleEMQX(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	switch request.URL.Path {
	case "/internal/emqx/auth":
		if s.hooks.Authenticate == nil {
			writeError(writer, http.StatusNotImplemented, errors.New("EMQX authentication is not configured"))
			return
		}
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
			ClientID string `json:"clientid"`
		}
		if err := readJSONLoose(request, &input); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		deviceID := strings.TrimSpace(input.Username)
		if deviceID == "" {
			deviceID = strings.TrimSpace(input.ClientID)
		}
		result, err := s.hooks.Authenticate(request.Context(), deviceID, input.Password)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err)
			return
		}
		response := map[string]any{"result": "deny"}
		if result.Allow {
			response["result"] = "allow"
			if len(result.ACL) > 0 {
				response["acl"] = result.ACL
			}
		}
		writeJSON(writer, http.StatusOK, response)
	case "/internal/emqx/webhook":
		if s.hooks.Lifecycle == nil {
			writeError(writer, http.StatusNotImplemented, errors.New("EMQX lifecycle webhook is not configured"))
			return
		}
		var input struct {
			Event     string `json:"event"`
			Action    string `json:"action"`
			ClientID  string `json:"clientid"`
			DeviceID  string `json:"device_id"`
			Timestamp int64  `json:"timestamp"`
		}
		if err := readJSONLoose(request, &input); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		event := strings.ToLower(strings.ReplaceAll(input.Event, "_", "."))
		if event == "" {
			event = strings.ToLower(strings.ReplaceAll(input.Action, "_", "."))
		}
		var online bool
		switch event {
		case "client.connected", "client.connect":
			online = true
		case "client.disconnected", "client.disconnect":
			online = false
		default:
			writeError(writer, http.StatusBadRequest, errors.New("event must be client.connected or client.disconnected"))
			return
		}
		deviceID := strings.TrimSpace(input.DeviceID)
		if deviceID == "" {
			deviceID = strings.TrimSpace(input.ClientID)
		}
		if deviceID == "" {
			writeError(writer, http.StatusBadRequest, errors.New("device_id or clientid is required"))
			return
		}
		at := time.Now().UTC()
		if input.Timestamp > 0 {
			at = time.UnixMilli(input.Timestamp).UTC()
		}
		if err := s.hooks.Lifecycle(request.Context(), deviceID, online, at); err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeError(writer, http.StatusNotFound, errors.New("route not found"))
	}
}

type productRequest struct {
	Name        string            `json:"name"`
	ProductKey  string            `json:"product_key"`
	Description string            `json:"description"`
	DeviceType  domain.DeviceType `json:"device_type"`
	Properties  []domain.Property `json:"properties"`
}

func (s *Server) handleProducts(writer http.ResponseWriter, request *http.Request, rest []string) {
	if len(rest) != 0 {
		writeError(writer, http.StatusNotFound, errors.New("route not found"))
		return
	}
	switch request.Method {
	case http.MethodPost:
		var input productRequest
		if err := readJSON(request, &input); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		if input.Name == "" {
			writeError(writer, http.StatusBadRequest, errors.New("name is required"))
			return
		}
		if input.ProductKey == "" {
			input.ProductKey = generatedID("pk")
		}
		if !validSegment(input.ProductKey) {
			writeError(writer, http.StatusBadRequest, errors.New("invalid product_key"))
			return
		}
		product, err := s.repos.Products.CreateProduct(request.Context(), domain.Product{
			Name: input.Name, ProductKey: input.ProductKey, Description: input.Description,
			DeviceType: input.DeviceType, Properties: input.Properties,
		})
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, product)
	case http.MethodGet:
		page, pageSize := pagination(request)
		products, meta, err := s.repos.Products.ListProducts(request.Context(), repository.ProductFilter{Page: page, PageSize: pageSize})
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"items": products, "page": meta})
	default:
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

type deviceRequest struct {
	DeviceID     string `json:"device_id"`
	ProductKey   string `json:"product_key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	DeviceSecret string `json:"device_secret"`
}

type deviceResponse struct {
	domain.Device
	DeviceSecret string `json:"device_secret,omitempty"`
}

func (s *Server) handleDevices(writer http.ResponseWriter, request *http.Request, rest []string) {
	if len(rest) == 0 {
		switch request.Method {
		case http.MethodPost:
			s.createDevice(writer, request)
		case http.MethodGet:
			s.listDevices(writer, request)
		default:
			writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
		return
	}
	deviceID := rest[0]
	if len(rest) == 1 {
		switch request.Method {
		case http.MethodGet:
			device, err := s.repos.Devices.GetDevice(request.Context(), deviceID)
			if err != nil {
				writeRepositoryError(writer, err)
				return
			}
			writeJSON(writer, http.StatusOK, device)
		case http.MethodDelete:
			if err := s.repos.Devices.SoftDeleteDevice(request.Context(), deviceID); err != nil {
				writeRepositoryError(writer, err)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
		return
	}
	if len(rest) == 2 && rest[1] == "telemetry" && request.Method == http.MethodGet {
		s.getTelemetry(writer, request, deviceID)
		return
	}
	if len(rest) == 2 && rest[1] == "snapshot" && request.Method == http.MethodGet {
		s.getSnapshot(writer, request, deviceID)
		return
	}
	if len(rest) == 2 && rest[1] == "shadow" {
		s.handleShadow(writer, request, deviceID, "")
		return
	}
	if len(rest) == 3 && rest[1] == "shadow" && rest[2] == "desired" && request.Method == http.MethodPut {
		s.handleShadow(writer, request, deviceID, "desired")
		return
	}
	if len(rest) == 2 && rest[1] == "commands" {
		s.handleCommands(writer, request, deviceID, "")
		return
	}
	if len(rest) == 3 && rest[1] == "commands" {
		s.handleCommands(writer, request, deviceID, rest[2])
		return
	}
	writeError(writer, http.StatusNotFound, errors.New("route not found"))
}

func (s *Server) createDevice(writer http.ResponseWriter, request *http.Request) {
	var input deviceRequest
	if err := readJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	if input.ProductKey == "" || input.Name == "" {
		writeError(writer, http.StatusBadRequest, errors.New("product_key and name are required"))
		return
	}
	if input.DeviceID == "" {
		input.DeviceID = generatedID("device")
	}
	if input.DeviceSecret == "" {
		input.DeviceSecret = generatedSecret()
	}
	device, err := s.repos.Devices.CreateDevice(request.Context(), domain.Device{
		DeviceID: input.DeviceID, ProductKey: input.ProductKey, Name: input.Name,
		Description: input.Description, DeviceSecret: input.DeviceSecret,
	})
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, deviceResponse{Device: device, DeviceSecret: input.DeviceSecret})
}

func (s *Server) listDevices(writer http.ResponseWriter, request *http.Request) {
	page, pageSize := pagination(request)
	devices, meta, err := s.repos.Devices.ListDevices(request.Context(), repository.DeviceFilter{
		ProductKey: request.URL.Query().Get("product_key"),
		Status:     domain.DeviceStatus(request.URL.Query().Get("status")), Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": devices, "page": meta})
}

func (s *Server) getTelemetry(writer http.ResponseWriter, request *http.Request, deviceID string) {
	limit := queryInt(request, "limit", 100)
	if limit > 10000 {
		limit = 10000
	}
	query := repository.TelemetryQuery{DeviceID: deviceID, Metric: request.URL.Query().Get("metric"), Limit: limit}
	query.From = unixTime(request.URL.Query().Get("from"))
	query.To = unixTime(request.URL.Query().Get("to"))
	items, err := s.repos.Telemetry.QueryTelemetry(request.Context(), query)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getSnapshot(writer http.ResponseWriter, request *http.Request, deviceID string) {
	snapshot, err := s.repos.Telemetry.SnapshotTelemetry(request.Context(), deviceID)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

func (s *Server) handleShadow(writer http.ResponseWriter, request *http.Request, deviceID, action string) {
	if action == "" && request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if action == "" {
		shadow, err := s.repos.Shadows.GetShadow(request.Context(), deviceID)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, shadow)
		return
	}
	var desired map[string]any
	if err := readJSON(request, &desired); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	shadow, err := s.repos.Shadows.UpsertDesired(request.Context(), deviceID, desired)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	device, err := s.repos.Devices.GetDevice(request.Context(), deviceID)
	if err == nil && s.publisher != nil {
		payload, _ := json.Marshal(desired)
		if err := s.publisher.Publish(request.Context(), messaging.DeviceTopic(device.ProductKey, deviceID, "shadow/desired"), messaging.QoSAtLeastOnce, false, payload); err != nil {
			writeError(writer, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(writer, http.StatusOK, shadow)
}

func (s *Server) handleCommands(writer http.ResponseWriter, request *http.Request, deviceID, commandID string) {
	if commandID != "" {
		if request.Method != http.MethodGet {
			writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		command, err := s.repos.Commands.GetCommand(request.Context(), deviceID, commandID)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, command)
		return
	}
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var input struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := readJSON(request, &input); err != nil || input.Method == "" {
		if err == nil {
			err = errors.New("method is required")
		}
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	device, err := s.repos.Devices.GetDevice(request.Context(), deviceID)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	command, err := s.repos.Commands.CreateCommand(request.Context(), domain.Command{DeviceID: deviceID, Method: input.Method, Params: input.Params})
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	if s.publisher != nil {
		payload, _ := json.Marshal(map[string]any{"command_id": command.ID, "method": input.Method, "params": input.Params})
		if err := s.publisher.Publish(request.Context(), messaging.DeviceTopic(device.ProductKey, deviceID, "command"), messaging.QoSAtLeastOnce, false, payload); err != nil {
			_ = s.repos.Commands.UpdateCommandStatus(request.Context(), command.ID, domain.CommandStatusFailed, err.Error(), time.Now().UTC())
			writeError(writer, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(writer, http.StatusAccepted, command)
}

type ruleRequest struct {
	ProductKey      string         `json:"product_key"`
	Name            string         `json:"name"`
	PropertyName    string         `json:"property_name"`
	Operator        string         `json:"operator"`
	Threshold       float64        `json:"threshold"`
	DurationSeconds int            `json:"duration_seconds"`
	ActionType      string         `json:"action_type"`
	ActionParams    map[string]any `json:"action_params"`
	Enabled         *bool          `json:"enabled"`
}

func (s *Server) handleRules(writer http.ResponseWriter, request *http.Request, rest []string) {
	if len(rest) != 0 {
		writeError(writer, http.StatusNotFound, errors.New("route not found"))
		return
	}
	switch request.Method {
	case http.MethodPost:
		var input ruleRequest
		if err := readJSON(request, &input); err != nil || input.ProductKey == "" || input.Name == "" || input.PropertyName == "" {
			if err == nil {
				err = errors.New("product_key, name, and property_name are required")
			}
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		rule, err := s.repos.Rules.CreateRule(request.Context(), domain.Rule{
			ProductKey: input.ProductKey, Name: input.Name, PropertyName: input.PropertyName,
			Operator: input.Operator, Threshold: input.Threshold, DurationSeconds: input.DurationSeconds,
			ActionType: input.ActionType, ActionParams: input.ActionParams, Enabled: enabled,
		})
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, rule)
	case http.MethodGet:
		productKey := request.URL.Query().Get("product_key")
		if productKey == "" {
			writeError(writer, http.StatusBadRequest, errors.New("product_key is required"))
			return
		}
		rules, err := s.repos.Rules.ListRulesByProduct(request.Context(), productKey)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"items": rules})
	default:
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (s *Server) handleAlarms(writer http.ResponseWriter, request *http.Request, rest []string) {
	if (len(rest) == 1 || len(rest) == 2 && rest[1] == "resolve") && request.Method == http.MethodPut && rest[0] != "" {
		var input struct {
			Note string `json:"note"`
		}
		if err := readJSON(request, &input); err != nil || strings.TrimSpace(input.Note) == "" {
			if err == nil {
				err = errors.New("note is required")
			}
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		if err := s.repos.Alarms.ResolveAlarm(request.Context(), rest[0], time.Now().UTC(), input.Note); err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if len(rest) != 0 || request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	page, pageSize := pagination(request)
	alarms, meta, err := s.repos.Alarms.ListAlarms(request.Context(), repository.AlarmFilter{
		DeviceID: request.URL.Query().Get("device_id"), ProductKey: request.URL.Query().Get("product_key"),
		Status: domain.AlarmStatus(request.URL.Query().Get("status")), From: unixTime(request.URL.Query().Get("from")),
		To: unixTime(request.URL.Query().Get("to")), Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": alarms, "page": meta})
}

func readJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func readJSONLoose(request *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, err error) {
	writeJSON(writer, status, map[string]any{"error": http.StatusText(status), "message": err.Error()})
}

func writeRepositoryError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, repository.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, repository.ErrConflict) {
		status = http.StatusConflict
	}
	writeError(writer, status, err)
}

func pathSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func validSegment(value string) bool {
	return value != "" && !strings.ContainsAny(value, "/+#")
}

func generatedID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func generatedSecret() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("sim-%028d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func pagination(request *http.Request) (int, int) {
	return queryInt(request, "page", 1), queryInt(request, "page_size", 20)
}

func queryInt(request *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(request.URL.Query().Get(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func unixTime(value string) time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}
