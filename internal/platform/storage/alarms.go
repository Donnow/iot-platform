package storage

import (
	"context"
	"strconv"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func (s *Store) CreateAlarm(ctx context.Context, alarm domain.Alarm) (domain.Alarm, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Alarm{}, err
	}
	if alarm.Status == "" {
		alarm.Status = domain.AlarmStatusActive
	}
	if alarm.TriggeredAt.IsZero() {
		alarm.TriggeredAt = time.Now().UTC()
	}
	var result domain.Alarm
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO alarms (device_id, rule_id, trigger_value, status, triggered_at)
		VALUES ($1, $2::uuid, $3, $4, $5)
		RETURNING id::text, device_id, rule_id::text, trigger_value, status, triggered_at, resolved_at, COALESCE(resolve_note, '')`,
		alarm.DeviceID, alarm.RuleID, alarm.TriggerValue, alarm.Status, alarm.TriggeredAt).Scan(
		&result.ID, &result.DeviceID, &result.RuleID, &result.TriggerValue, &result.Status,
		&result.TriggeredAt, &result.ResolvedAt, &result.ResolveNote)
	if err != nil {
		return domain.Alarm{}, mapDBError(err)
	}
	return result, nil
}

func (s *Store) GetAlarm(ctx context.Context, alarmID string) (domain.Alarm, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Alarm{}, err
	}
	var alarm domain.Alarm
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, device_id, rule_id::text, trigger_value, status, triggered_at, resolved_at, COALESCE(resolve_note, '')
		FROM alarms WHERE id = $1::uuid`, alarmID).Scan(
		&alarm.ID, &alarm.DeviceID, &alarm.RuleID, &alarm.TriggerValue, &alarm.Status,
		&alarm.TriggeredAt, &alarm.ResolvedAt, &alarm.ResolveNote)
	if err != nil {
		return domain.Alarm{}, mapDBError(err)
	}
	return alarm, nil
}

func (s *Store) ListAlarms(ctx context.Context, filter repository.AlarmFilter) ([]domain.Alarm, repository.Page, error) {
	if err := contextErr(ctx); err != nil {
		return nil, repository.Page{}, err
	}
	page, pageSize, offset := pageValues(filter.Page, filter.PageSize)
	where, args := alarmWhere(filter)
	from := `FROM alarms a JOIN devices d ON d.device_id = a.device_id JOIN products p ON p.id = d.product_id WHERE ` + where
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+from, args...).Scan(&total); err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id::text, a.device_id, a.rule_id::text, a.trigger_value, a.status, a.triggered_at, a.resolved_at, COALESCE(a.resolve_note, '') `+
		from+` ORDER BY a.triggered_at DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	defer rows.Close()
	alarms := make([]domain.Alarm, 0)
	for rows.Next() {
		var alarm domain.Alarm
		if err := rows.Scan(&alarm.ID, &alarm.DeviceID, &alarm.RuleID, &alarm.TriggerValue, &alarm.Status,
			&alarm.TriggeredAt, &alarm.ResolvedAt, &alarm.ResolveNote); err != nil {
			return nil, repository.Page{}, mapDBError(err)
		}
		alarms = append(alarms, alarm)
	}
	if err := rows.Err(); err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	return alarms, repository.Page{Page: page, PageSize: pageSize, Total: total}, nil
}

func alarmWhere(filter repository.AlarmFilter) (string, []any) {
	where := []string{"1 = 1"}
	args := make([]any, 0, 6)
	add := func(expression string, value any) {
		args = append(args, value)
		where = append(where, expression+" $"+strconv.Itoa(len(args)))
	}
	if filter.DeviceID != "" {
		add("a.device_id =", filter.DeviceID)
	}
	if filter.ProductKey != "" {
		add("p.product_key =", filter.ProductKey)
	}
	if filter.Status != "" {
		add("a.status =", filter.Status)
	}
	if !filter.From.IsZero() {
		add("a.triggered_at >=", filter.From)
	}
	if !filter.To.IsZero() {
		add("a.triggered_at <=", filter.To)
	}
	return joinAnd(where), args
}

func (s *Store) ResolveAlarm(ctx context.Context, alarmID string, resolvedAt time.Time, note string) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE alarms SET status = $2, resolved_at = $3, resolve_note = $4 WHERE id = $1::uuid`,
		alarmID, domain.AlarmStatusResolved, resolvedAt, note)
	if err != nil {
		return mapDBError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return mapDBError(err)
	}
	if count == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func joinAnd(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += " AND "
		}
		result += value
	}
	return result
}

var _ repository.AlarmRepository = (*Store)(nil)
