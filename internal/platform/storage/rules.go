package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/repository"
)

func (s *Store) CreateRule(ctx context.Context, rule domain.Rule) (domain.Rule, error) {
	if err := contextErr(ctx); err != nil {
		return domain.Rule{}, err
	}
	productID, err := s.productID(ctx, rule.ProductKey)
	if err != nil {
		return domain.Rule{}, err
	}
	params, err := json.Marshal(rule.ActionParams)
	if err != nil {
		return domain.Rule{}, fmt.Errorf("encode rule action params: %w", err)
	}
	var result domain.Rule
	var rawParams []byte
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO rules (product_id, name, property_name, operator, threshold, duration_seconds, action_type, action_params, enabled)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
		RETURNING id::text, name, property_name, operator, threshold, duration_seconds, action_type, action_params, enabled, created_at`,
		productID, rule.Name, rule.PropertyName, rule.Operator, rule.Threshold, rule.DurationSeconds,
		rule.ActionType, params, rule.Enabled).Scan(
		&result.ID, &result.Name, &result.PropertyName, &result.Operator, &result.Threshold,
		&result.DurationSeconds, &result.ActionType, &rawParams, &result.Enabled, &result.CreatedAt)
	if err != nil {
		return domain.Rule{}, mapDBError(err)
	}
	result.ProductKey = rule.ProductKey
	if len(rawParams) > 0 && string(rawParams) != "null" {
		if err := json.Unmarshal(rawParams, &result.ActionParams); err != nil {
			return domain.Rule{}, fmt.Errorf("decode rule action params: %w", err)
		}
	}
	return result, nil
}

func (s *Store) ListRulesByProduct(ctx context.Context, productKey string) ([]domain.Rule, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id::text, p.product_key, r.name, r.property_name, r.operator, r.threshold,
		       r.duration_seconds, r.action_type, r.action_params, r.enabled, r.created_at
		FROM rules r JOIN products p ON p.id = r.product_id
		WHERE p.product_key = $1 ORDER BY r.created_at ASC`, productKey)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	result := make([]domain.Rule, 0)
	for rows.Next() {
		var rule domain.Rule
		var rawParams []byte
		if err := rows.Scan(&rule.ID, &rule.ProductKey, &rule.Name, &rule.PropertyName, &rule.Operator,
			&rule.Threshold, &rule.DurationSeconds, &rule.ActionType, &rawParams, &rule.Enabled, &rule.CreatedAt); err != nil {
			return nil, mapDBError(err)
		}
		if len(rawParams) > 0 && string(rawParams) != "null" {
			if err := json.Unmarshal(rawParams, &rule.ActionParams); err != nil {
				return nil, fmt.Errorf("decode rule action params: %w", err)
			}
		}
		result = append(result, rule)
	}
	return result, mapDBError(rows.Err())
}

func (s *Store) productID(ctx context.Context, productKey string) (string, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT id::text FROM products WHERE product_key = $1`, productKey).Scan(&id); err != nil {
		return "", mapDBError(err)
	}
	return id, nil
}

var _ repository.RuleRepository = (*Store)(nil)
