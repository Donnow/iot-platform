package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/redis/go-redis/v9"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func (s *Store) GetShadow(ctx context.Context, deviceID string) (domain.Shadow, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Shadow{}, err
	}
	if cached, err := getShadowCache(ctx, s.redis, deviceID); err == nil {
		var shadow domain.Shadow
		if err := json.Unmarshal(cached, &shadow); err == nil {
			return normalizeShadow(shadow), nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return domain.Shadow{}, err
	}
	var shadow domain.Shadow
	var desired, reported []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT desired, reported, updated_at FROM device_shadows WHERE device_id = $1`, deviceID).Scan(
		&desired, &reported, &shadow.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return emptyShadow(deviceID), nil
	}
	if err != nil {
		return domain.Shadow{}, mapDBError(err)
	}
	shadow.DeviceID = deviceID
	if err := json.Unmarshal(desired, &shadow.Desired); err != nil {
		return domain.Shadow{}, fmt.Errorf("decode desired shadow: %w", err)
	}
	if err := json.Unmarshal(reported, &shadow.Reported); err != nil {
		return domain.Shadow{}, fmt.Errorf("decode reported shadow: %w", err)
	}
	shadow = normalizeShadow(shadow)
	encoded, _ := json.Marshal(shadow)
	_ = setShadowCache(ctx, s.redis, deviceID, encoded)
	return shadow, nil
}

func (s *Store) UpsertDesired(ctx context.Context, deviceID string, desired map[string]any) (domain.Shadow, error) {
	return s.upsertShadow(ctx, deviceID, desired, true)
}

func (s *Store) UpsertReported(ctx context.Context, deviceID string, reported map[string]any) (domain.Shadow, error) {
	return s.upsertShadow(ctx, deviceID, reported, false)
}

func (s *Store) upsertShadow(ctx context.Context, deviceID string, values map[string]any, desired bool) (domain.Shadow, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Shadow{}, err
	}
	shadow, err := s.GetShadow(ctx, deviceID)
	if err != nil {
		return domain.Shadow{}, err
	}
	if desired {
		shadow.Desired = mergeShadowMap(shadow.Desired, values)
	} else {
		shadow.Reported = mergeShadowMap(shadow.Reported, values)
	}
	shadow.DeviceID = deviceID
	shadow.UpdatedAt = time.Now().UTC()
	shadow = normalizeShadow(shadow)
	desiredJSON, err := json.Marshal(shadow.Desired)
	if err != nil {
		return domain.Shadow{}, fmt.Errorf("encode desired shadow: %w", err)
	}
	reportedJSON, err := json.Marshal(shadow.Reported)
	if err != nil {
		return domain.Shadow{}, fmt.Errorf("encode reported shadow: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO device_shadows (device_id, desired, reported, updated_at)
		VALUES ($1, $2::jsonb, $3::jsonb, $4)
		ON CONFLICT (device_id) DO UPDATE SET desired = EXCLUDED.desired, reported = EXCLUDED.reported, updated_at = EXCLUDED.updated_at`,
		deviceID, desiredJSON, reportedJSON, shadow.UpdatedAt)
	if err != nil {
		return domain.Shadow{}, mapDBError(err)
	}
	encoded, _ := json.Marshal(shadow)
	if err := setShadowCache(ctx, s.redis, deviceID, encoded); err != nil {
		return domain.Shadow{}, err
	}
	return shadow, nil
}

func normalizeShadow(shadow domain.Shadow) domain.Shadow {
	if shadow.Desired == nil {
		shadow.Desired = map[string]any{}
	}
	if shadow.Reported == nil {
		shadow.Reported = map[string]any{}
	}
	shadow.Delta = shadowDelta(shadow.Desired, shadow.Reported)
	return cloneShadow(shadow)
}

func emptyShadow(deviceID string) domain.Shadow {
	return domain.Shadow{DeviceID: deviceID, Desired: map[string]any{}, Reported: map[string]any{}, Delta: map[string]any{}}
}

func mergeShadowMap(base, patch map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(patch))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range patch {
		result[key] = value
	}
	return result
}

func shadowDelta(desired, reported map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range desired {
		if reportedValue, exists := reported[key]; !exists || !reflect.DeepEqual(reportedValue, value) {
			result[key] = value
		}
	}
	return result
}

func cloneShadow(shadow domain.Shadow) domain.Shadow {
	copy := shadow
	copy.Desired = mergeShadowMap(nil, shadow.Desired)
	copy.Reported = mergeShadowMap(nil, shadow.Reported)
	copy.Delta = mergeShadowMap(nil, shadow.Delta)
	return copy
}

var _ repository.ShadowRepository = (*Store)(nil)
