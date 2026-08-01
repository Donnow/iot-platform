CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(64) NOT NULL,
    product_key VARCHAR(32) UNIQUE NOT NULL,
    device_type VARCHAR(16) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS product_properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    name VARCHAR(64) NOT NULL,
    data_type VARCHAR(16) NOT NULL,
    unit VARCHAR(16),
    min_value DOUBLE PRECISION,
    max_value DOUBLE PRECISION,
    UNIQUE(product_id, name)
);

CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id VARCHAR(64) UNIQUE NOT NULL,
    device_secret VARCHAR(64) NOT NULL,
    product_id UUID NOT NULL REFERENCES products(id),
    name VARCHAR(64) NOT NULL,
    description TEXT,
    status VARCHAR(16) NOT NULL DEFAULT 'inactive',
    last_online TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    name VARCHAR(64) NOT NULL,
    property_name VARCHAR(64) NOT NULL,
    operator VARCHAR(4) NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    duration_seconds INT NOT NULL DEFAULT 0,
    action_type VARCHAR(16) NOT NULL,
    action_params JSONB,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alarms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id VARCHAR(64) NOT NULL,
    rule_id UUID NOT NULL REFERENCES rules(id),
    trigger_value DOUBLE PRECISION NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    resolve_note TEXT
);

CREATE TABLE IF NOT EXISTS commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id VARCHAR(64) NOT NULL,
    method VARCHAR(64) NOT NULL,
    params JSONB,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS device_shadows (
    device_id VARCHAR(64) PRIMARY KEY,
    desired JSONB NOT NULL DEFAULT '{}',
    reported JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS firmwares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    version VARCHAR(32) NOT NULL,
    md5 VARCHAR(32) NOT NULL,
    file_url TEXT NOT NULL,
    changelog TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(product_id, version),
    CHECK (md5 ~ '^[0-9a-fA-F]{32}$')
);

CREATE TABLE IF NOT EXISTS ota_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    firmware_id UUID NOT NULL REFERENCES firmwares(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ota_task_devices (
    task_id UUID NOT NULL REFERENCES ota_tasks(id) ON DELETE CASCADE,
    device_id VARCHAR(64) NOT NULL REFERENCES devices(device_id),
    stage VARCHAR(16) NOT NULL DEFAULT 'pending',
    progress INT NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    message TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(task_id, device_id),
    CHECK (stage IN ('pending', 'downloading', 'installing', 'success', 'failed'))
);
