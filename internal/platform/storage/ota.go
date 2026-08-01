package storage

import (
	"context"
	"errors"
	"sort"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func (s *Store) CreateFirmware(ctx context.Context, firmware domain.Firmware) (domain.Firmware, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Firmware{}, err
	}
	productID, err := s.productID(ctx, firmware.ProductKey)
	if err != nil {
		return domain.Firmware{}, err
	}
	var result domain.Firmware
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO firmwares (product_id, version, md5, file_url, changelog)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id::text, version, md5, file_url, COALESCE(changelog, ''), created_at`,
		productID, firmware.Version, firmware.MD5, firmware.FileURL, firmware.Changelog).Scan(
		&result.ID, &result.Version, &result.MD5, &result.FileURL, &result.Changelog, &result.CreatedAt)
	if err != nil {
		return domain.Firmware{}, mapDBError(err)
	}
	result.ProductKey = firmware.ProductKey
	return result, nil
}

func (s *Store) GetFirmware(ctx context.Context, firmwareID string) (domain.Firmware, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Firmware{}, err
	}
	var firmware domain.Firmware
	err := s.db.QueryRowContext(ctx, `
		SELECT f.id::text, p.product_key, f.version, f.md5, f.file_url, COALESCE(f.changelog, ''), f.created_at
		FROM firmwares f JOIN products p ON p.id = f.product_id WHERE f.id = $1::uuid`, firmwareID).Scan(
		&firmware.ID, &firmware.ProductKey, &firmware.Version, &firmware.MD5, &firmware.FileURL, &firmware.Changelog, &firmware.CreatedAt)
	if err != nil {
		return domain.Firmware{}, mapDBError(err)
	}
	return firmware, nil
}

func (s *Store) ListFirmwares(ctx context.Context, productKey string) ([]domain.Firmware, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	query := `
		SELECT f.id::text, p.product_key, f.version, f.md5, f.file_url, COALESCE(f.changelog, ''), f.created_at
		FROM firmwares f JOIN products p ON p.id = f.product_id`
	args := []any{}
	if productKey != "" {
		query += " WHERE p.product_key = $1"
		args = append(args, productKey)
	}
	query += " ORDER BY f.created_at ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	result := make([]domain.Firmware, 0)
	for rows.Next() {
		var firmware domain.Firmware
		if err := rows.Scan(&firmware.ID, &firmware.ProductKey, &firmware.Version, &firmware.MD5,
			&firmware.FileURL, &firmware.Changelog, &firmware.CreatedAt); err != nil {
			return nil, mapDBError(err)
		}
		result = append(result, firmware)
	}
	return result, mapDBError(rows.Err())
}

func (s *Store) CreateOTATask(ctx context.Context, task domain.OTATask) (domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return domain.OTATask{}, err
	}
	if len(task.TargetDeviceIDs) == 0 {
		return domain.OTATask{}, errors.New("OTA task needs at least one target device")
	}
	productID, err := s.productID(ctx, task.ProductKey)
	if err != nil {
		return domain.OTATask{}, err
	}
	firmware, err := s.GetFirmware(ctx, task.FirmwareID)
	if err != nil {
		return domain.OTATask{}, err
	}
	if firmware.ProductKey != task.ProductKey || firmware.Version != task.Version {
		return domain.OTATask{}, repository.ErrNotFound
	}
	seen := make(map[string]struct{}, len(task.TargetDeviceIDs))
	for _, deviceID := range task.TargetDeviceIDs {
		if _, exists := seen[deviceID]; exists {
			return domain.OTATask{}, repository.ErrConflict
		}
		seen[deviceID] = struct{}{}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.OTATask{}, mapDBError(err)
	}
	defer tx.Rollback()
	for _, deviceID := range task.TargetDeviceIDs {
		var found string
		err := tx.QueryRowContext(ctx, `
			SELECT device_id FROM devices WHERE device_id = $1 AND product_id = $2::uuid AND status <> 'deleted'`,
			deviceID, productID).Scan(&found)
		if err != nil {
			return domain.OTATask{}, mapDBError(err)
		}
	}
	now := time.Now().UTC()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = now
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO ota_tasks (product_id, firmware_id, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4) RETURNING id::text`,
		productID, task.FirmwareID, task.CreatedAt, task.UpdatedAt).Scan(&task.ID)
	if err != nil {
		return domain.OTATask{}, mapDBError(err)
	}
	for _, deviceID := range task.TargetDeviceIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ota_task_devices (task_id, device_id, stage, progress, updated_at)
			VALUES ($1::uuid, $2, $3, 0, $4)`, task.ID, deviceID, domain.OTAStagePending, task.UpdatedAt); err != nil {
			return domain.OTATask{}, mapDBError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.OTATask{}, mapDBError(err)
	}
	return s.GetOTATask(ctx, task.ID)
}

func (s *Store) GetOTATask(ctx context.Context, taskID string) (domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return domain.OTATask{}, err
	}
	return s.loadOTATask(ctx, taskID)
}

func (s *Store) ListOTATasks(ctx context.Context, productKey string) ([]domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	query := `
		SELECT t.id::text FROM ota_tasks t JOIN products p ON p.id = t.product_id`
	args := []any{}
	if productKey != "" {
		query += " WHERE p.product_key = $1"
		args = append(args, productKey)
	}
	query += " ORDER BY t.created_at ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, mapDBError(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err)
	}
	result := make([]domain.OTATask, 0, len(ids))
	for _, id := range ids {
		task, err := s.loadOTATask(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, nil
}

func (s *Store) UpdateOTAProgress(ctx context.Context, taskID, deviceID, stage string, progress int, message string, updatedAt time.Time) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if progress < 0 || progress > 100 {
		return errors.New("OTA progress must be between 0 and 100")
	}
	if !validOTAStage(stage) {
		return errors.New("unsupported OTA stage")
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE ota_task_devices SET stage = $3, progress = $4, message = $5, updated_at = $6
		WHERE task_id = $1::uuid AND device_id = $2`, taskID, deviceID, stage, progress, message, updatedAt)
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
	if _, err := s.db.ExecContext(ctx, `UPDATE ota_tasks SET updated_at = $2 WHERE id = $1::uuid`, taskID, updatedAt); err != nil {
		return mapDBError(err)
	}
	return nil
}

func (s *Store) ListPendingOTA(ctx context.Context, deviceID string) ([]domain.OTATask, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT task_id::text FROM ota_task_devices
		WHERE device_id = $1 AND stage NOT IN ('success', 'failed') ORDER BY updated_at ASC`, deviceID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, mapDBError(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err)
	}
	result := make([]domain.OTATask, 0, len(ids))
	for _, id := range ids {
		task, err := s.loadOTATask(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) loadOTATask(ctx context.Context, taskID string) (domain.OTATask, error) {
	var task domain.OTATask
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id::text, p.product_key, f.id::text, f.version, f.file_url, f.md5, t.created_at, t.updated_at
		FROM ota_tasks t JOIN products p ON p.id = t.product_id JOIN firmwares f ON f.id = t.firmware_id
		WHERE t.id = $1::uuid`, taskID).Scan(
		&task.ID, &task.ProductKey, &task.FirmwareID, &task.Version, &task.URL, &task.MD5, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return domain.OTATask{}, mapDBError(err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT device_id, stage, progress, COALESCE(message, ''), updated_at
		FROM ota_task_devices WHERE task_id = $1::uuid ORDER BY device_id`, taskID)
	if err != nil {
		return domain.OTATask{}, mapDBError(err)
	}
	defer rows.Close()
	task.TargetDeviceIDs = make([]string, 0)
	task.Progress = make([]domain.OTADeviceProgress, 0)
	task.Summary = make(map[domain.OTAStage]int)
	for rows.Next() {
		var item domain.OTADeviceProgress
		var stage string
		if err := rows.Scan(&item.DeviceID, &stage, &item.Progress, &item.Message, &item.UpdatedAt); err != nil {
			return domain.OTATask{}, mapDBError(err)
		}
		item.Stage = domain.OTAStage(stage)
		task.TargetDeviceIDs = append(task.TargetDeviceIDs, item.DeviceID)
		task.Progress = append(task.Progress, item)
		task.Summary[item.Stage]++
	}
	if err := rows.Err(); err != nil {
		return domain.OTATask{}, mapDBError(err)
	}
	return task, nil
}

func validOTAStage(stage string) bool {
	switch domain.OTAStage(stage) {
	case domain.OTAStagePending, domain.OTAStageDownloading, domain.OTAStageInstalling, domain.OTAStageSuccess, domain.OTAStageFailed:
		return true
	default:
		return false
	}
}

var _ repository.OTARepository = (*Store)(nil)
