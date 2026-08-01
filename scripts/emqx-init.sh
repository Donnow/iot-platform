#!/bin/sh
# Bootstrap EMQX data-integration resources for the iot-perform platform.
#
# EMQX Open Source 5.8 removed the built-in Webhooks feature (Enterprise only),
# so device lifecycle events are forwarded through the rule engine to an HTTP
# bridge pointing at the platform's /internal/emqx/webhook endpoint.
#
# This script is idempotent: resources already present are left untouched,
# which makes it safe to re-run on every `docker compose up`.

set -eu

API="${EMQX_API:-http://emqx:18083/api/v5}"
USERNAME="${EMQX_DASHBOARD_USER:-admin}"
PASSWORD="${EMQX_DASHBOARD_PASSWORD:-public}"
WEBHOOK_URL="${IOT_WEBHOOK_URL:-http://platform:8080/internal/emqx/webhook}"

log() {
    echo "[emqx-init] $*"
}

wait_for_api() {
    i=0
    while [ "$i" -lt 60 ]; do
        if curl -fsS -o /dev/null "${API%/}/status" 2>/dev/null; then
            return 0
        fi
        i=$((i + 1))
        sleep 2
    done
    echo "[emqx-init] EMQX API did not become ready" >&2
    exit 1
}

auth_header() {
    curl -fsS -X POST "${API%/}/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}" |
        sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
}

create_if_missing() {
    check_path="$1"
    create_path="$2"
    body="$3"
    if curl -fsS -o /dev/null "${API%/}/${check_path}" -H "$AUTH" 2>/dev/null; then
        log "${check_path} already exists, skipping"
        return
    fi
    if curl -fsS -X POST "${API%/}/${create_path}" -H "$AUTH" \
        -H 'Content-Type: application/json' -d "$body" -o /dev/null; then
        log "${check_path} created"
    else
        echo "[emqx-init] failed to create ${check_path}" >&2
        exit 1
    fi
}

wait_for_api
AUTH="Authorization: Bearer $(auth_header)"

create_if_missing "connectors/http:iot_webhook_connector" "connectors" \
    "{\"type\":\"http\",\"name\":\"iot_webhook_connector\",\"url\":\"${WEBHOOK_URL}\",\"headers\":{\"content-type\":\"application/json\"},\"pool_size\":4}"

create_if_missing "bridges/webhook:iot_webhook" "bridges" \
    "{\"type\":\"http\",\"name\":\"iot_webhook\",\"enable\":true,\"url\":\"${WEBHOOK_URL}\",\"method\":\"post\",\"body\":\"{\\\"event\\\":\\\"\\\${event}\\\",\\\"clientid\\\":\\\"\\\${clientid}\\\"}\",\"headers\":{\"content-type\":\"application/json\"}}"

create_if_missing "rules/iot_client_connected" "rules" \
    "{\"id\":\"iot_client_connected\",\"sql\":\"SELECT clientid, event FROM \\\"\\\$events/client_connected\\\"\",\"actions\":[\"http:iot_webhook\"],\"enable\":true}"

create_if_missing "rules/iot_client_disconnected" "rules" \
    "{\"id\":\"iot_client_disconnected\",\"sql\":\"SELECT clientid, event FROM \\\"\\\$events/client_disconnected\\\"\",\"actions\":[\"http:iot_webhook\"],\"enable\":true}"

log "done"
