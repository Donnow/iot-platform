#!/usr/bin/env bash
# TDengine 单普通表 -> 超级表/子表 迁移脚本
#
# 背景:旧版本遥测落在单普通表 iot_telemetry.telemetry(ts 为主键,同毫秒多设备
# 相互覆盖),新版本改为超级表 iot_telemetry.telemetry + 每设备子表
# t_<md5(device_id) 前 8 字节 hex>。超级表与旧普通表同名,必须先改名再建表。
#
# 步骤:
#   1. RENAME TABLE iot_telemetry.telemetry -> iot_telemetry.telemetry_legacy
#   2. 建超级表(CREATE STABLE IF NOT EXISTS,含 DESCRIBE 结构校验)
#   3. 重放:按 device_id 分组,INSERT ... USING ... TAGS 写入子表(自动建表,幂等)
#   4. 校验:设备数 / 每设备样本数 / 时间范围 / 抽样逐条比对 ts+payload
#   5. 校验通过后按需 DROP TABLE telemetry_legacy(--drop-legacy 才执行,默认保留)
#
# 幂等可重跑:任何一步中断后重新运行会从断点继续;校验失败绝不删除 legacy 表。
# 注意:旧表已被覆盖的数据无法恢复,重放只搬现存数据。
# 迁移前请暂停平台写流量(旧平台写 iot_telemetry.telemetry,改名后写入会失败)。
#
# 用法: scripts/tdengine-migrate.sh [--drop-legacy]
# 环境变量(与平台一致):
#   IOT_TDENGINE_URL       默认 http://localhost:6041
#   IOT_TDENGINE_USERNAME  默认 root
#   IOT_TDENGINE_PASSWORD  默认 taosdata

set -euo pipefail

usage() {
  cat <<'EOF'
用法: scripts/tdengine-migrate.sh [选项]

选项:
  --drop-legacy   校验通过后 DROP TABLE iot_telemetry.telemetry_legacy(默认保留)
  -h, --help      显示本帮助

环境变量(与平台一致):
  IOT_TDENGINE_URL        默认 http://localhost:6041
  IOT_TDENGINE_USERNAME   默认 root
  IOT_TDENGINE_PASSWORD   默认 taosdata

前置条件:
  - TDengine REST 接口(6041)可用
  - 迁移期间暂停平台写流量(建议停止 platform 服务或模拟器)
EOF
}

DROP_LEGACY=0
for arg in "$@"; do
  case "$arg" in
    --drop-legacy) DROP_LEGACY=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知选项: $arg" >&2; usage; exit 2 ;;
  esac
done

export IOT_TDENGINE_URL="${IOT_TDENGINE_URL:-http://localhost:6041}"
export IOT_TDENGINE_USERNAME="${IOT_TDENGINE_USERNAME:-root}"
export IOT_TDENGINE_PASSWORD="${IOT_TDENGINE_PASSWORD:-taosdata}"
export IOT_MIGRATE_DROP_LEGACY="$DROP_LEGACY"

python3 - <<'PY'
import base64
import hashlib
import json
import os
import sys
import urllib.error
import urllib.request

TD_URL = os.environ["IOT_TDENGINE_URL"].rstrip("/")
TD_USER = os.environ["IOT_TDENGINE_USERNAME"]
TD_PASS = os.environ["IOT_TDENGINE_PASSWORD"]
DROP_LEGACY = os.environ["IOT_MIGRATE_DROP_LEGACY"] == "1"

DB = "iot_telemetry"
STABLE = f"{DB}.telemetry"
LEGACY = f"{DB}.telemetry_legacy"
LEGACY_SHORT = "telemetry_legacy"


def log(message):
    print(f"[migrate] {message}", flush=True)


def sql(statement):
    request = urllib.request.Request(
        f"{TD_URL}/rest/sql",
        data=statement.encode("utf-8"),
        headers={"Content-Type": "text/plain"},
        method="POST",
    )
    if TD_USER:
        token = base64.b64encode(f"{TD_USER}:{TD_PASS}".encode()).decode()
        request.add_header("Authorization", f"Basic {token}")
    try:
        with urllib.request.urlopen(request, timeout=120) as response:
            body = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"TDengine HTTP {error.code}: {error.reason}") from error
    if body.get("code", -1) != 0:
        raise RuntimeError(
            f"TDengine error {body.get('code')}: {body.get('desc')} (statement: {statement})"
        )
    return body


def scalar(statement):
    data = sql(statement).get("data") or []
    if not data:
        return None
    return data[0][0]


def escape_sql(value):
    return str(value).replace("\\", "\\\\").replace("'", "''")


def table_exists(name):
    count = scalar(
        f"SELECT COUNT(*) FROM information_schema.ins_tables "
        f"WHERE db_name = '{DB}' AND table_name = '{name}'"
    )
    return int(count or 0) > 0


def stable_exists():
    count = scalar(
        f"SELECT COUNT(*) FROM information_schema.ins_stables "
        f"WHERE db_name = '{DB}' AND stable_name = 'telemetry'"
    )
    return int(count or 0) > 0


def child_table(device_id):
    digest = hashlib.md5(device_id.encode("utf-8")).hexdigest()
    return f"{DB}.t_" + digest[:16]


def ensure_stable():
    sql(f"CREATE DATABASE IF NOT EXISTS {DB} KEEP 3650")
    sql(
        f"CREATE STABLE IF NOT EXISTS {STABLE} "
        f"(ts TIMESTAMP, payload NCHAR(4096)) "
        f"TAGS (device_id BINARY(128), product_key BINARY(128))"
    )
    describe = sql(f"DESCRIBE {STABLE}")
    tagged = [row for row in describe.get("data") or [] if len(row) > 3 and row[3] == "TAG"]
    fields = {row[0]: row[1] for row in describe.get("data") or [] if row}
    if "ts" not in fields or "payload" not in fields:
        raise RuntimeError(f"DESCRIBE {STABLE} 缺少 ts/payload 列: {describe.get('data')}")
    if {"device_id", "product_key"} - {row[0] for row in tagged}:
        raise RuntimeError(f"DESCRIBE {STABLE} 缺少 device_id/product_key 标签: {describe.get('data')}")


def sampled_ts(device_id, legacy_count):
    samples = []
    for offset in (0, max(0, legacy_count // 2), max(0, legacy_count - 1)):
        row = sql(
            f"SELECT CAST(ts AS BIGINT) FROM {LEGACY} WHERE device_id = '{escape_sql(device_id)}' "
            f"ORDER BY ts ASC LIMIT 1 OFFSET {offset}"
        ).get("data")
        if row:
            samples.append(int(row[0][0]))
    return samples


def replay_device(device_id, product_key):
    child = child_table(device_id)
    legacy_count = int(
        scalar(f"SELECT COUNT(*) FROM {LEGACY} WHERE device_id = '{escape_sql(device_id)}'") or 0
    )
    offset = 0
    batch = 500
    while offset < legacy_count:
        rows = sql(
            f"SELECT CAST(ts AS BIGINT), payload FROM {LEGACY} WHERE device_id = '{escape_sql(device_id)}' "
            f"ORDER BY ts ASC LIMIT {batch} OFFSET {offset}"
        ).get("data") or []
        values = [f"({int(ts)}, '{escape_sql(payload)}')" for ts, payload in rows]
        if values:
            statement = (
                f"INSERT INTO {child} USING {STABLE} "
                f"TAGS ('{escape_sql(device_id)}', '{escape_sql(product_key)}') VALUES "
                + " ".join(values)
            )
            sql(statement)
        offset += batch

    child_count = int(scalar(f"SELECT COUNT(*) FROM {child}") or 0)
    if child_count != legacy_count:
        raise RuntimeError(
            f"{device_id}: 样本数不一致 legacy={legacy_count} child={child_count}"
        )
    legacy_min = scalar(f"SELECT FIRST(ts) FROM {LEGACY} WHERE device_id = '{escape_sql(device_id)}'")
    legacy_max = scalar(f"SELECT LAST(ts) FROM {LEGACY} WHERE device_id = '{escape_sql(device_id)}'")
    child_min = scalar(f"SELECT FIRST(ts) FROM {child}")
    child_max = scalar(f"SELECT LAST(ts) FROM {child}")
    if child_min != legacy_min or child_max != legacy_max:
        raise RuntimeError(
            f"{device_id}: 时间范围不一致 legacy=[{legacy_min},{legacy_max}] "
            f"child=[{child_min},{child_max}]"
        )
    for ts in sampled_ts(device_id, legacy_count):
        legacy_row = sql(
            f"SELECT CAST(ts AS BIGINT), payload FROM {LEGACY} "
            f"WHERE device_id = '{escape_sql(device_id)}' AND ts = {ts}"
        ).get("data")
        child_row = sql(
            f"SELECT CAST(ts AS BIGINT), payload FROM {child} WHERE ts = {ts}"
        ).get("data")
        if not legacy_row and not child_row:
            continue
        if legacy_row != child_row:
            raise RuntimeError(f"{device_id}: ts={ts} 抽样比对不一致 legacy={legacy_row} child={child_row}")
    return legacy_count


def replay_and_verify():
    legacy_total = int(scalar(f"SELECT COUNT(*) FROM {LEGACY}") or 0)
    devices = sql(f"SELECT DISTINCT device_id, product_key FROM {LEGACY}").get("data") or []
    log(f"legacy 表共 {legacy_total} 条, {len(devices)} 个设备")

    migrated = 0
    for device_id, product_key in devices:
        count = replay_device(str(device_id), str(product_key))
        migrated += count
    if migrated != legacy_total:
        raise RuntimeError(f"迁移行数不一致: 重放 {migrated} != legacy {legacy_total}")
    log(f"重放完成: {migrated} 条 / {len(devices)} 设备,计数、时间范围、抽样比对全部通过")


def main():
    log(f"TDengine: {TD_URL}")
    sql("SELECT 1")

    legacy_exists = table_exists(LEGACY_SHORT)
    if not legacy_exists:
        if stable_exists():
            log(f"{STABLE} 已是超级表且无 legacy 表,迁移已完成,跳过")
            return
        if not table_exists("telemetry"):
            log(f"未发现旧表 {STABLE},仅确保超级表结构")
            ensure_stable()
            log("完成(无数据可迁移)")
            return
        log("[1/6] 重命名旧普通表 -> telemetry_legacy")
        sql(f"RENAME TABLE {STABLE} TO {LEGACY}")
    else:
        log("[1/6] 已存在 telemetry_legacy,跳过 RENAME(续跑)")

    log("[2/6] 确保数据库与超级表结构")
    ensure_stable()

    log("[3/6] 重放 legacy 数据到子表")
    replay_and_verify()

    log("[4/6] 校验通过")
    if DROP_LEGACY:
        log("[5/6] --drop-legacy 已指定,删除 legacy 表")
        sql(f"DROP TABLE {LEGACY}")
        log("完成:已删除 iot_telemetry.telemetry_legacy")
    else:
        log("[5/6] 保留 legacy 表(未传 --drop-legacy)")
        log("确认无误后手动删除(或重跑脚本加 --drop-legacy):")
        log(f"  curl -u {TD_USER}:*** -X POST {TD_URL}/rest/sql -d 'DROP TABLE {LEGACY}'")


if __name__ == "__main__":
    main()
PY
