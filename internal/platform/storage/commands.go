package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func (s *Store) CreateCommand(ctx context.Context, command domain.Command) (domain.Command, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Command{}, err
	}
	if _, err := s.GetDevice(ctx, command.DeviceID); err != nil {
		return domain.Command{}, err
	}
	params, err := json.Marshal(command.Params)
	if err != nil {
		return domain.Command{}, fmt.Errorf("encode command params: %w", err)
	}
	now := time.Now().UTC()
	if command.CreatedAt.IsZero() {
		command.CreatedAt = now
	}
	if command.UpdatedAt.IsZero() {
		command.UpdatedAt = now
	}
	if command.Status == "" {
		command.Status = domain.CommandStatusPending
	}
	var result domain.Command
	var rawParams []byte
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO commands (device_id, method, params, status, message, created_at, updated_at)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7)
		RETURNING id::text, device_id, method, params, status, COALESCE(message, ''), created_at, updated_at`,
		command.DeviceID, command.Method, params, command.Status, command.Message, command.CreatedAt, command.UpdatedAt).Scan(
		&result.ID, &result.DeviceID, &result.Method, &rawParams, &result.Status, &result.Message,
		&result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return domain.Command{}, mapDBError(err)
	}
	if len(rawParams) > 0 && string(rawParams) != "null" {
		if err := json.Unmarshal(rawParams, &result.Params); err != nil {
			return domain.Command{}, fmt.Errorf("decode command params: %w", err)
		}
	}
	return result, nil
}

func (s *Store) GetCommand(ctx context.Context, deviceID, commandID string) (domain.Command, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Command{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE commands SET status = $1, updated_at = now()
		WHERE id = $2::uuid AND device_id = $3 AND status = $4 AND created_at < now() - interval '30 seconds'`,
		domain.CommandStatusTimeout, commandID, deviceID, domain.CommandStatusPending); err != nil {
		return domain.Command{}, mapDBError(err)
	}
	var command domain.Command
	var rawParams []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, device_id, method, params, status, COALESCE(message, ''), created_at, updated_at
		FROM commands WHERE id = $1::uuid AND device_id = $2`, commandID, deviceID).Scan(
		&command.ID, &command.DeviceID, &command.Method, &rawParams, &command.Status, &command.Message,
		&command.CreatedAt, &command.UpdatedAt)
	if err != nil {
		return domain.Command{}, mapDBError(err)
	}
	if len(rawParams) > 0 && string(rawParams) != "null" {
		if err := json.Unmarshal(rawParams, &command.Params); err != nil {
			return domain.Command{}, fmt.Errorf("decode command params: %w", err)
		}
	}
	return command, nil
}

func (s *Store) UpdateCommandStatus(ctx context.Context, commandID string, status domain.CommandStatus, message string, updatedAt time.Time) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE commands SET status = $2, message = $3, updated_at = $4 WHERE id = $1::uuid`,
		commandID, status, message, updatedAt)
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

var _ repository.CommandRepository = (*Store)(nil)
