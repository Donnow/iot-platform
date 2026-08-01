package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func (s *Store) CreateProduct(ctx context.Context, product domain.Product) (domain.Product, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Product{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Product{}, mapDBError(err)
	}
	defer tx.Rollback()
	var id string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO products (name, product_key, device_type, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, created_at`, product.Name, product.ProductKey, product.DeviceType, product.Description).Scan(&id, &createdAt)
	if err != nil {
		return domain.Product{}, mapDBError(err)
	}
	for _, property := range product.Properties {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_properties (product_id, name, data_type, unit, min_value, max_value)
			VALUES ($1::uuid, $2, $3, $4, $5, $6)`, id, property.Name, property.DataType, property.Unit, property.MinValue, property.MaxValue); err != nil {
			return domain.Product{}, mapDBError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Product{}, mapDBError(err)
	}
	product.ID = id
	product.CreatedAt = createdAt
	return cloneProduct(product), nil
}

func (s *Store) GetProductByKey(ctx context.Context, productKey string) (domain.Product, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Product{}, err
	}
	var product domain.Product
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, name, product_key, COALESCE(description, ''), device_type, created_at
		FROM products WHERE product_key = $1`, productKey).Scan(&product.ID, &product.Name, &product.ProductKey, &product.Description, &product.DeviceType, &product.CreatedAt)
	if err != nil {
		return domain.Product{}, mapDBError(err)
	}
	product.Properties, err = s.productProperties(ctx, product.ID)
	if err != nil {
		return domain.Product{}, err
	}
	return product, nil
}

func (s *Store) ListProducts(ctx context.Context, filter repository.ProductFilter) ([]domain.Product, repository.Page, error) {
	if err := contextErr(ctx); err != nil {
		return nil, repository.Page{}, err
	}
	page, pageSize, offset := pageValues(filter.Page, filter.PageSize)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products`).Scan(&total); err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id::text, p.name, p.product_key, COALESCE(p.description, ''), p.device_type,
		       p.created_at, COUNT(d.device_id) FILTER (WHERE d.status = 'online')
		FROM products p
		LEFT JOIN devices d ON d.product_id = p.id
		GROUP BY p.id
		ORDER BY p.created_at ASC
		LIMIT $1 OFFSET $2`, pageSize, offset)
	if err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	defer rows.Close()
	products := make([]domain.Product, 0)
	for rows.Next() {
		var product domain.Product
		if err := rows.Scan(&product.ID, &product.Name, &product.ProductKey, &product.Description, &product.DeviceType, &product.CreatedAt, &product.OnlineDeviceCount); err != nil {
			return nil, repository.Page{}, mapDBError(err)
		}
		product.Properties, err = s.productProperties(ctx, product.ID)
		if err != nil {
			return nil, repository.Page{}, err
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	return products, repository.Page{Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Store) productProperties(ctx context.Context, productID string) ([]domain.Property, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, data_type, COALESCE(unit, ''), min_value, max_value
		FROM product_properties WHERE product_id = $1::uuid ORDER BY id`, productID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	properties := make([]domain.Property, 0)
	for rows.Next() {
		var property domain.Property
		var minValue, maxValue sql.NullFloat64
		if err := rows.Scan(&property.Name, &property.DataType, &property.Unit, &minValue, &maxValue); err != nil {
			return nil, mapDBError(err)
		}
		if minValue.Valid {
			property.MinValue = &minValue.Float64
		}
		if maxValue.Valid {
			property.MaxValue = &maxValue.Float64
		}
		properties = append(properties, property)
	}
	return properties, rows.Err()
}

func (s *Store) CreateDevice(ctx context.Context, device domain.Device) (domain.Device, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Device{}, err
	}
	var productID string
	if err := s.db.QueryRowContext(ctx, `SELECT id::text FROM products WHERE product_key = $1`, device.ProductKey).Scan(&productID); err != nil {
		return domain.Device{}, mapDBError(err)
	}
	var id string
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO devices (device_id, device_secret, product_id, name, description)
		VALUES ($1, $2, $3::uuid, $4, $5)
		RETURNING id::text, created_at`, device.DeviceID, device.DeviceSecret, productID, device.Name, device.Description).Scan(&id, &createdAt)
	if err != nil {
		return domain.Device{}, mapDBError(err)
	}
	device.ID = id
	device.CreatedAt = createdAt
	if device.Status == "" {
		device.Status = domain.DeviceStatusInactive
	}
	return device, nil
}

const deviceSelect = `
	SELECT d.id::text, d.device_id, d.device_secret, p.product_key, d.name,
	       COALESCE(d.description, ''), d.status, d.last_online, d.created_at
	FROM devices d JOIN products p ON p.id = d.product_id`

func (s *Store) GetDevice(ctx context.Context, deviceID string) (domain.Device, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Device{}, err
	}
	var device domain.Device
	err := s.db.QueryRowContext(ctx, deviceSelect+` WHERE d.device_id = $1`, deviceID).Scan(
		&device.ID, &device.DeviceID, &device.DeviceSecret, &device.ProductKey, &device.Name,
		&device.Description, &device.Status, &device.LastOnline, &device.CreatedAt)
	if err != nil {
		return domain.Device{}, mapDBError(err)
	}
	return device, nil
}

func (s *Store) ListDevices(ctx context.Context, filter repository.DeviceFilter) ([]domain.Device, repository.Page, error) {
	if err := contextErr(ctx); err != nil {
		return nil, repository.Page{}, err
	}
	page, pageSize, offset := pageValues(filter.Page, filter.PageSize)
	args := make([]any, 0, 4)
	where := []string{"d.status <> 'deleted'"}
	if filter.ProductKey != "" {
		args = append(args, filter.ProductKey)
		where = append(where, fmt.Sprintf("p.product_key = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("d.status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices d JOIN products p ON p.id = d.product_id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(ctx, deviceSelect+` WHERE `+whereSQL+` ORDER BY d.created_at ASC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	defer rows.Close()
	devices := make([]domain.Device, 0)
	for rows.Next() {
		var device domain.Device
		if err := rows.Scan(&device.ID, &device.DeviceID, &device.DeviceSecret, &device.ProductKey, &device.Name, &device.Description, &device.Status, &device.LastOnline, &device.CreatedAt); err != nil {
			return nil, repository.Page{}, mapDBError(err)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, repository.Page{}, mapDBError(err)
	}
	return devices, repository.Page{Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Store) SetDeviceStatus(ctx context.Context, deviceID string, status domain.DeviceStatus, onlineAt *time.Time) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE devices
		SET status = $2::varchar,
			last_online = CASE
				WHEN $2::varchar = 'online' THEN COALESCE($3::timestamptz, last_online)
				ELSE last_online
			END
		WHERE device_id = $1 AND status <> 'deleted'`, deviceID, status, onlineAt)
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
	return mapDBError(setOnlineCache(ctx, s.redis, deviceID, status == domain.DeviceStatusOnline))
}

func (s *Store) SoftDeleteDevice(ctx context.Context, deviceID string) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE devices SET status = 'deleted' WHERE device_id = $1 AND status <> 'deleted'`, deviceID)
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
	return mapDBError(setOnlineCache(ctx, s.redis, deviceID, false))
}

func (s *Store) AuthenticateDevice(ctx context.Context, deviceID, secret string) (domain.Device, error) {
	device, err := s.GetDevice(ctx, deviceID)
	if err != nil || device.Status == domain.DeviceStatusDeleted || device.DeviceSecret != secret {
		return domain.Device{}, repository.ErrNotFound
	}
	return device, nil
}

func cloneProduct(product domain.Product) domain.Product {
	product.Properties = append([]domain.Property(nil), product.Properties...)
	return product
}

func scanJSON(raw any, target any) error {
	var data []byte
	switch value := raw.(type) {
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(data, target)
}
