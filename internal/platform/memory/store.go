package memory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

var (
	ErrNotFound = repository.ErrNotFound
	ErrConflict = repository.ErrConflict
)

type Store struct {
	mu        sync.RWMutex
	sequence  uint64
	users     map[string]domain.User
	products  map[string]domain.Product
	devices   map[string]domain.Device
	telemetry map[string][]domain.Telemetry
	rules     map[string]domain.Rule
	alarms    map[string]domain.Alarm
	commands  map[string]domain.Command
	shadows   map[string]domain.Shadow
	firmwares map[string]domain.Firmware
	otaTasks  map[string]domain.OTATask
	otaState  map[string]map[string]domain.OTADeviceProgress
}

func New() *Store {
	return &Store{
		users:     make(map[string]domain.User),
		products:  make(map[string]domain.Product),
		devices:   make(map[string]domain.Device),
		telemetry: make(map[string][]domain.Telemetry),
		rules:     make(map[string]domain.Rule),
		alarms:    make(map[string]domain.Alarm),
		commands:  make(map[string]domain.Command),
		shadows:   make(map[string]domain.Shadow),
		firmwares: make(map[string]domain.Firmware),
		otaTasks:  make(map[string]domain.OTATask),
		otaState:  make(map[string]map[string]domain.OTADeviceProgress),
	}
}

func (s *Store) Repositories() repository.Repositories {
	return repository.Repositories{
		Users:     s,
		Products:  s,
		Devices:   s,
		Telemetry: s,
		Rules:     s,
		Alarms:    s,
		Commands:  s,
		Shadows:   s,
		OTA:       s,
	}
}

func (s *Store) CreateProduct(ctx context.Context, product domain.Product) (domain.Product, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Product{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.products[product.ProductKey]; exists {
		return domain.Product{}, ErrConflict
	}
	if product.ID == "" {
		product.ID = s.nextID("product")
	}
	if product.CreatedAt.IsZero() {
		product.CreatedAt = time.Now().UTC()
	}
	product.Properties = cloneProperties(product.Properties)
	s.products[product.ProductKey] = product
	return cloneProduct(product), nil
}

func (s *Store) GetProductByKey(ctx context.Context, productKey string) (domain.Product, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Product{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	product, exists := s.products[productKey]
	if !exists {
		return domain.Product{}, ErrNotFound
	}
	return cloneProduct(product), nil
}

func (s *Store) ListProducts(ctx context.Context, filter repository.ProductFilter) ([]domain.Product, repository.Page, error) {
	if err := contextErr(ctx); err != nil {
		return nil, repository.Page{}, err
	}
	s.mu.RLock()
	products := make([]domain.Product, 0, len(s.products))
	for _, product := range s.products {
		for _, device := range s.devices {
			if device.ProductKey == product.ProductKey && device.Status == domain.DeviceStatusOnline {
				product.OnlineDeviceCount++
			}
		}
		products = append(products, cloneProduct(product))
	}
	s.mu.RUnlock()
	sort.Slice(products, func(i, j int) bool { return products[i].CreatedAt.Before(products[j].CreatedAt) })
	page, items := paginate(products, filter.Page, filter.PageSize)
	return items, page, nil
}

func (s *Store) CreateDevice(ctx context.Context, device domain.Device) (domain.Device, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Device{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.devices[device.DeviceID]; exists {
		return domain.Device{}, ErrConflict
	}
	if _, exists := s.products[device.ProductKey]; !exists {
		return domain.Device{}, fmt.Errorf("product %q: %w", device.ProductKey, ErrNotFound)
	}
	if device.ID == "" {
		device.ID = s.nextID("device")
	}
	if device.Status == "" {
		device.Status = domain.DeviceStatusInactive
	}
	if device.CreatedAt.IsZero() {
		device.CreatedAt = time.Now().UTC()
	}
	s.devices[device.DeviceID] = device
	return cloneDevice(device), nil
}

func (s *Store) GetDevice(ctx context.Context, deviceID string) (domain.Device, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Device{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	device, exists := s.devices[deviceID]
	if !exists {
		return domain.Device{}, ErrNotFound
	}
	return cloneDevice(device), nil
}

func (s *Store) ListDevices(ctx context.Context, filter repository.DeviceFilter) ([]domain.Device, repository.Page, error) {
	if err := contextErr(ctx); err != nil {
		return nil, repository.Page{}, err
	}
	s.mu.RLock()
	devices := make([]domain.Device, 0, len(s.devices))
	for _, device := range s.devices {
		if device.Status == domain.DeviceStatusDeleted {
			continue
		}
		if filter.ProductKey != "" && device.ProductKey != filter.ProductKey {
			continue
		}
		if filter.Status != "" && device.Status != filter.Status {
			continue
		}
		devices = append(devices, cloneDevice(device))
	}
	s.mu.RUnlock()
	sort.Slice(devices, func(i, j int) bool { return devices[i].CreatedAt.Before(devices[j].CreatedAt) })
	page, items := paginate(devices, filter.Page, filter.PageSize)
	return items, page, nil
}

func (s *Store) SetDeviceStatus(ctx context.Context, deviceID string, status domain.DeviceStatus, onlineAt *time.Time) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	device, exists := s.devices[deviceID]
	if !exists || device.Status == domain.DeviceStatusDeleted {
		return ErrNotFound
	}
	device.Status = status
	if onlineAt != nil {
		timestamp := onlineAt.UTC()
		device.LastOnline = &timestamp
	}
	s.devices[deviceID] = device
	return nil
}

func (s *Store) SoftDeleteDevice(ctx context.Context, deviceID string) error {
	return s.SetDeviceStatus(ctx, deviceID, domain.DeviceStatusDeleted, nil)
}

func (s *Store) AuthenticateDevice(ctx context.Context, deviceID, secret string) (domain.Device, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Device{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	device, exists := s.devices[deviceID]
	if !exists || device.Status == domain.DeviceStatusDeleted || device.DeviceSecret != secret {
		return domain.Device{}, ErrNotFound
	}
	return cloneDevice(device), nil
}

func (s *Store) AppendTelemetry(ctx context.Context, sample domain.Telemetry) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if sample.DeviceID == "" || sample.Timestamp.IsZero() || len(sample.Values) == 0 {
		return errors.New("telemetry requires device, timestamp, and values")
	}
	s.mu.Lock()
	s.telemetry[sample.DeviceID] = append(s.telemetry[sample.DeviceID], cloneTelemetry(sample))
	s.mu.Unlock()
	return nil
}

func (s *Store) QueryTelemetry(ctx context.Context, query repository.TelemetryQuery) ([]domain.Telemetry, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	samples := append([]domain.Telemetry(nil), s.telemetry[query.DeviceID]...)
	s.mu.RUnlock()
	result := make([]domain.Telemetry, 0, len(samples))
	for _, sample := range samples {
		if !query.From.IsZero() && sample.Timestamp.Before(query.From) {
			continue
		}
		if !query.To.IsZero() && sample.Timestamp.After(query.To) {
			continue
		}
		if query.Metric != "" {
			value, ok := sample.Values[query.Metric]
			if !ok {
				continue
			}
			sample.Values = map[string]any{query.Metric: value}
		}
		result = append(result, cloneTelemetry(sample))
	}
	if query.Interval != "" && query.Interval != "raw" {
		result = aggregateTelemetry(result, query.Interval)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	if query.Limit > 0 && len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result, nil
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
	default:
		return 0, false
	}
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

func (s *Store) CreateRule(ctx context.Context, rule domain.Rule) (domain.Rule, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Rule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.products[rule.ProductKey]; !exists {
		return domain.Rule{}, ErrNotFound
	}
	if rule.ID == "" {
		rule.ID = s.nextID("rule")
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	s.rules[rule.ID] = cloneRule(rule)
	return cloneRule(rule), nil
}

func (s *Store) ListRulesByProduct(ctx context.Context, productKey string) ([]domain.Rule, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Rule, 0)
	for _, rule := range s.rules {
		if rule.ProductKey == productKey {
			result = append(result, cloneRule(rule))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) CreateAlarm(ctx context.Context, alarm domain.Alarm) (domain.Alarm, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Alarm{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if alarm.ID == "" {
		alarm.ID = s.nextID("alarm")
	}
	if alarm.Status == "" {
		alarm.Status = domain.AlarmStatusActive
	}
	if alarm.TriggeredAt.IsZero() {
		alarm.TriggeredAt = time.Now().UTC()
	}
	s.alarms[alarm.ID] = alarm
	return alarm, nil
}

func (s *Store) GetAlarm(ctx context.Context, alarmID string) (domain.Alarm, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Alarm{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	alarm, exists := s.alarms[alarmID]
	if !exists {
		return domain.Alarm{}, ErrNotFound
	}
	return alarm, nil
}

func (s *Store) ListAlarms(ctx context.Context, filter repository.AlarmFilter) ([]domain.Alarm, repository.Page, error) {
	if err := contextErr(ctx); err != nil {
		return nil, repository.Page{}, err
	}
	s.mu.RLock()
	alarms := make([]domain.Alarm, 0, len(s.alarms))
	for _, alarm := range s.alarms {
		if filter.ProductKey != "" {
			device, exists := s.devices[alarm.DeviceID]
			if !exists || device.ProductKey != filter.ProductKey {
				continue
			}
		}
		if filter.DeviceID != "" && alarm.DeviceID != filter.DeviceID {
			continue
		}
		if filter.Status != "" && alarm.Status != filter.Status {
			continue
		}
		if !filter.From.IsZero() && alarm.TriggeredAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && alarm.TriggeredAt.After(filter.To) {
			continue
		}
		alarms = append(alarms, alarm)
	}
	s.mu.RUnlock()
	sort.Slice(alarms, func(i, j int) bool { return alarms[i].TriggeredAt.After(alarms[j].TriggeredAt) })
	page, items := paginate(alarms, filter.Page, filter.PageSize)
	return items, page, nil
}

func (s *Store) ResolveAlarm(ctx context.Context, alarmID string, resolvedAt time.Time, note string) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	alarm, exists := s.alarms[alarmID]
	if !exists {
		return ErrNotFound
	}
	alarm.Status = domain.AlarmStatusResolved
	alarm.ResolvedAt = &resolvedAt
	alarm.ResolveNote = note
	s.alarms[alarmID] = alarm
	return nil
}

func (s *Store) CreateCommand(ctx context.Context, command domain.Command) (domain.Command, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Command{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.devices[command.DeviceID]; !exists {
		return domain.Command{}, ErrNotFound
	}
	if command.ID == "" {
		command.ID = s.nextID("command")
	}
	if command.Status == "" {
		command.Status = domain.CommandStatusPending
	}
	now := time.Now().UTC()
	if command.CreatedAt.IsZero() {
		command.CreatedAt = now
	}
	if command.UpdatedAt.IsZero() {
		command.UpdatedAt = now
	}
	s.commands[command.ID] = cloneCommand(command)
	return cloneCommand(command), nil
}

func (s *Store) GetCommand(ctx context.Context, deviceID, commandID string) (domain.Command, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Command{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	command, exists := s.commands[commandID]
	if !exists || command.DeviceID != deviceID {
		return domain.Command{}, ErrNotFound
	}
	if command.Status == domain.CommandStatusPending && time.Since(command.CreatedAt) > 30*time.Second {
		command.Status = domain.CommandStatusTimeout
		command.UpdatedAt = time.Now().UTC()
		s.commands[commandID] = command
	}
	return cloneCommand(command), nil
}

func (s *Store) UpdateCommandStatus(ctx context.Context, commandID string, status domain.CommandStatus, message string, updatedAt time.Time) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	command, exists := s.commands[commandID]
	if !exists {
		return ErrNotFound
	}
	command.Status = status
	command.Message = message
	command.UpdatedAt = updatedAt
	s.commands[commandID] = command
	return nil
}

func (s *Store) GetShadow(ctx context.Context, deviceID string) (domain.Shadow, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Shadow{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	shadow, exists := s.shadows[deviceID]
	if !exists {
		return domain.Shadow{DeviceID: deviceID, Desired: map[string]any{}, Reported: map[string]any{}, Delta: map[string]any{}}, nil
	}
	return cloneShadow(shadow), nil
}

func (s *Store) UpsertDesired(ctx context.Context, deviceID string, desired map[string]any) (domain.Shadow, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Shadow{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	shadow := s.shadows[deviceID]
	shadow.DeviceID = deviceID
	shadow.Desired = mergeMap(shadow.Desired, desired)
	shadow.UpdatedAt = time.Now().UTC()
	shadow.Delta = delta(shadow.Desired, shadow.Reported)
	s.shadows[deviceID] = shadow
	return cloneShadow(shadow), nil
}

func (s *Store) UpsertReported(ctx context.Context, deviceID string, reported map[string]any) (domain.Shadow, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Shadow{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	shadow := s.shadows[deviceID]
	shadow.DeviceID = deviceID
	shadow.Reported = mergeMap(shadow.Reported, reported)
	shadow.UpdatedAt = time.Now().UTC()
	shadow.Delta = delta(shadow.Desired, shadow.Reported)
	s.shadows[deviceID] = shadow
	return cloneShadow(shadow), nil
}

func (s *Store) CreateFirmware(ctx context.Context, firmware domain.Firmware) (domain.Firmware, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Firmware{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.products[firmware.ProductKey]; !exists {
		return domain.Firmware{}, ErrNotFound
	}
	for _, existing := range s.firmwares {
		if existing.ProductKey == firmware.ProductKey && existing.Version == firmware.Version {
			return domain.Firmware{}, ErrConflict
		}
	}
	if firmware.ID == "" {
		firmware.ID = s.nextID("firmware")
	}
	if firmware.CreatedAt.IsZero() {
		firmware.CreatedAt = time.Now().UTC()
	}
	s.firmwares[firmware.ID] = firmware
	return firmware, nil
}

func (s *Store) GetFirmware(ctx context.Context, firmwareID string) (domain.Firmware, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Firmware{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	firmware, exists := s.firmwares[firmwareID]
	if !exists {
		return domain.Firmware{}, ErrNotFound
	}
	return firmware, nil
}

func (s *Store) ListFirmwares(ctx context.Context, productKey string) ([]domain.Firmware, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	result := make([]domain.Firmware, 0, len(s.firmwares))
	for _, firmware := range s.firmwares {
		if productKey == "" || firmware.ProductKey == productKey {
			result = append(result, firmware)
		}
	}
	s.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) CreateOTATask(ctx context.Context, task domain.OTATask) (domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return domain.OTATask{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	firmware, exists := s.firmwares[task.FirmwareID]
	if !exists || firmware.ProductKey != task.ProductKey || firmware.Version != task.Version {
		return domain.OTATask{}, ErrNotFound
	}
	if len(task.TargetDeviceIDs) == 0 {
		return domain.OTATask{}, errors.New("OTA task needs at least one target device")
	}
	seen := make(map[string]struct{}, len(task.TargetDeviceIDs))
	for _, deviceID := range task.TargetDeviceIDs {
		if _, duplicate := seen[deviceID]; duplicate {
			return domain.OTATask{}, ErrConflict
		}
		device, exists := s.devices[deviceID]
		if !exists || device.ProductKey != task.ProductKey || device.Status == domain.DeviceStatusDeleted {
			return domain.OTATask{}, ErrNotFound
		}
		seen[deviceID] = struct{}{}
	}
	if task.ID == "" {
		task.ID = s.nextID("ota")
	}
	now := time.Now().UTC()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = now
	}
	s.otaTasks[task.ID] = cloneOTATask(task)
	s.otaState[task.ID] = make(map[string]domain.OTADeviceProgress, len(task.TargetDeviceIDs))
	for _, deviceID := range task.TargetDeviceIDs {
		s.otaState[task.ID][deviceID] = domain.OTADeviceProgress{DeviceID: deviceID, Stage: domain.OTAStagePending, UpdatedAt: now}
	}
	return s.otaTaskLocked(task.ID), nil
}

func (s *Store) GetOTATask(ctx context.Context, taskID string) (domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return domain.OTATask{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, exists := s.otaTasks[taskID]; !exists {
		return domain.OTATask{}, ErrNotFound
	}
	return s.otaTaskLocked(taskID), nil
}

func (s *Store) ListOTATasks(ctx context.Context, productKey string) ([]domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	result := make([]domain.OTATask, 0, len(s.otaTasks))
	for id, task := range s.otaTasks {
		if productKey == "" || task.ProductKey == productKey {
			result = append(result, s.otaTaskLocked(id))
		}
	}
	s.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) UpdateOTAProgress(ctx context.Context, taskID, deviceID, stage string, progress int, message string, updatedAt time.Time) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if progress < 0 || progress > 100 {
		return errors.New("OTA progress must be between 0 and 100")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.otaTasks[taskID]; !exists {
		return ErrNotFound
	}
	if _, exists := s.otaState[taskID][deviceID]; !exists {
		return ErrNotFound
	}
	parsedStage := domain.OTAStage(stage)
	switch parsedStage {
	case domain.OTAStagePending, domain.OTAStageDownloading, domain.OTAStageInstalling, domain.OTAStageSuccess, domain.OTAStageFailed:
	default:
		return errors.New("unsupported OTA stage")
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	s.otaState[taskID][deviceID] = domain.OTADeviceProgress{DeviceID: deviceID, Stage: parsedStage, Progress: progress, Message: message, UpdatedAt: updatedAt}
	task := s.otaTasks[taskID]
	task.UpdatedAt = updatedAt
	s.otaTasks[taskID] = task
	return nil
}

func (s *Store) ListPendingOTA(ctx context.Context, deviceID string) ([]domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	result := make([]domain.OTATask, 0)
	for id := range s.otaTasks {
		progress, ok := s.otaState[id][deviceID]
		if ok && progress.Stage != domain.OTAStageSuccess && progress.Stage != domain.OTAStageFailed {
			result = append(result, s.otaTaskLocked(id))
		}
	}
	s.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) otaTaskLocked(taskID string) domain.OTATask {
	task := cloneOTATask(s.otaTasks[taskID])
	task.Progress = task.Progress[:0]
	task.Summary = make(map[domain.OTAStage]int)
	for _, progress := range s.otaState[taskID] {
		task.Progress = append(task.Progress, progress)
		task.Summary[progress.Stage]++
	}
	sort.Slice(task.Progress, func(i, j int) bool { return task.Progress[i].DeviceID < task.Progress[j].DeviceID })
	return task
}

func (s *Store) nextID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddUint64(&s.sequence, 1))
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func paginate[T any](items []T, pageNumber, pageSize int) (repository.Page, []T) {
	if pageNumber < 1 {
		pageNumber = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	page := repository.Page{Page: pageNumber, PageSize: pageSize, Total: len(items)}
	start := (pageNumber - 1) * pageSize
	if start >= len(items) {
		return page, []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return page, items[start:end]
}

func mergeMap(base, updates map[string]any) map[string]any {
	result := cloneAnyMap(base)
	if result == nil {
		result = make(map[string]any)
	}
	for key, value := range updates {
		result[key] = value
	}
	return result
}

func delta(desired, reported map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range desired {
		if reported == nil || !reflect.DeepEqual(value, reported[key]) {
			result[key] = value
		}
	}
	return result
}

func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneProduct(product domain.Product) domain.Product {
	product.Properties = cloneProperties(product.Properties)
	return product
}

func cloneProperties(properties []domain.Property) []domain.Property {
	return append([]domain.Property(nil), properties...)
}

func cloneDevice(device domain.Device) domain.Device {
	if device.LastOnline != nil {
		timestamp := *device.LastOnline
		device.LastOnline = &timestamp
	}
	return device
}

func cloneTelemetry(sample domain.Telemetry) domain.Telemetry {
	sample.Values = cloneAnyMap(sample.Values)
	return sample
}

func cloneRule(rule domain.Rule) domain.Rule {
	rule.ActionParams = cloneAnyMap(rule.ActionParams)
	return rule
}

func cloneCommand(command domain.Command) domain.Command {
	command.Params = cloneAnyMap(command.Params)
	return command
}

func cloneShadow(shadow domain.Shadow) domain.Shadow {
	shadow.Desired = cloneAnyMap(shadow.Desired)
	shadow.Reported = cloneAnyMap(shadow.Reported)
	shadow.Delta = cloneAnyMap(shadow.Delta)
	return shadow
}

func cloneOTATask(task domain.OTATask) domain.OTATask {
	task.TargetDeviceIDs = append([]string(nil), task.TargetDeviceIDs...)
	task.Progress = append([]domain.OTADeviceProgress(nil), task.Progress...)
	task.Summary = make(map[domain.OTAStage]int, len(task.Summary))
	for stage, count := range task.Summary {
		task.Summary[stage] = count
	}
	return task
}

var _ repository.Repositories = repository.Repositories{}
var _ repository.ProductRepository = (*Store)(nil)
var _ repository.DeviceRepository = (*Store)(nil)
var _ repository.TelemetryRepository = (*Store)(nil)
var _ repository.RuleRepository = (*Store)(nil)
var _ repository.AlarmRepository = (*Store)(nil)
var _ repository.CommandRepository = (*Store)(nil)
var _ repository.ShadowRepository = (*Store)(nil)
var _ repository.OTARepository = (*Store)(nil)

func (s *Store) ProductCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.products)
}

func (s *Store) DeviceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.devices)
}

func normalizeStatus(status domain.DeviceStatus) domain.DeviceStatus {
	return domain.DeviceStatus(strings.ToLower(string(status)))
}
