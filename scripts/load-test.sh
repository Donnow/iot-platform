#!/usr/bin/env bash
# 一键压测脚本:创建产品 -> 注册设备 -> 生成凭据 -> 启动设备模拟器
# 用法: scripts/load-test.sh [选项]
# 选项:
#   -api <url>           平台 API 地址(默认 http://localhost:8080)
#   -token <jwt>         管理员 JWT(默认取环境变量 IOT_API_TOKEN)
#   -count <n>           设备数量(默认 1000)
#   -interval <d>        上报间隔(默认 5s)
#   -product-key <k>     产品 key(默认 stress-product)
#   -type <t>            设备类型: temperature|smoke|door|air-conditioner
#   -prefix <p>          设备 ID 前缀(默认 stress)
#   -broker <url>        MQTT broker(默认 tcp://localhost:1883)
#   -creds <file>        凭据输出文件(默认 ./creds-stress.json)
#   -concurrency <n>     注册并发数(默认 32)
#   -no-sim               只注册设备并生成凭据,不启动模拟器
set -euo pipefail

API=http://localhost:8080
TOKEN="${IOT_API_TOKEN:-}"
COUNT=1000
INTERVAL=5s
PRODUCT_KEY=stress-product
DEVICE_TYPE=temperature
PREFIX=stress
BROKER=tcp://localhost:1883
CREDS=./creds-stress.json
CONCURRENCY=32
NO_SIM=0

while [ $# -gt 0 ]; do
  case "$1" in
    -api) API="$2"; shift 2 ;;
    -token) TOKEN="$2"; shift 2 ;;
    -count) COUNT="$2"; shift 2 ;;
    -interval) INTERVAL="$2"; shift 2 ;;
    -product-key) PRODUCT_KEY="$2"; shift 2 ;;
    -type) DEVICE_TYPE="$2"; shift 2 ;;
    -prefix) PREFIX="$2"; shift 2 ;;
    -broker) BROKER="$2"; shift 2 ;;
    -creds) CREDS="$2"; shift 2 ;;
    -concurrency) CONCURRENCY="$2"; shift 2 ;;
    -no-sim) NO_SIM=1; shift 1 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

if [ -z "$TOKEN" ]; then
  echo "错误:需要 JWT。通过 -token 传入,或设置环境变量 IOT_API_TOKEN。" >&2
  exit 1
fi
AUTH="Authorization: Bearer $TOKEN"

case "$DEVICE_TYPE" in
  temperature)    DEVICE_TYPE_API=sensor;    PROPERTIES='[{"name":"temperature","data_type":"float","min_value":15,"max_value":45},{"name":"humidity","data_type":"float","min_value":30,"max_value":90}]' ;;
  smoke)          DEVICE_TYPE_API=sensor;    PROPERTIES='[{"name":"smoke_level","data_type":"float","min_value":0,"max_value":100}]' ;;
  door)           DEVICE_TYPE_API=actuator;  PROPERTIES='[{"name":"door_status","data_type":"string"}]' ;;
  air-conditioner) DEVICE_TYPE_API=composite; PROPERTIES='[{"name":"current_temp","data_type":"float","min_value":16,"max_value":30},{"name":"target_temp","data_type":"float","min_value":16,"max_value":30},{"name":"mode","data_type":"string"}]' ;;
  *) echo "不支持的类型: $DEVICE_TYPE" >&2; exit 2 ;;
esac

echo "==> 1/4 创建产品 $PRODUCT_KEY (type=$DEVICE_TYPE)"
CREATE_PRODUCT=$(curl -sf -X POST "$API/api/products" -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"name\":\"stress-$PRODUCT_KEY\",\"product_key\":\"$PRODUCT_KEY\",\"device_type\":\"$DEVICE_TYPE_API\",\"properties\":$PROPERTIES}" 2>/dev/null) || true
if [ -z "$CREATE_PRODUCT" ]; then
  if curl -sf "$API/api/products?product_key=$PRODUCT_KEY" -H "$AUTH" | grep -q "\"product_key\":\"$PRODUCT_KEY\""; then
    echo "   产品已存在,复用。"
  else
    echo "   创建失败,且未找到已有产品,终止。" >&2
    exit 1
  fi
else
  echo "   创建成功。"
fi

echo "==> 2/4 注册 $COUNT 台设备 (前缀 $PREFIX-,并发 $CONCURRENCY)"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
export API AUTH PREFIX PRODUCT_KEY

register_one() {
  local i=$1
  local id
  id=$(printf "%s-%04d" "$PREFIX" "$i")
  local body
  body=$(curl -sf -X POST "$API/api/devices" -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"product_key\":\"$PRODUCT_KEY\",\"device_id\":\"$id\",\"name\":\"$id\"}") || {
    # ID 已存在或冲突时重试一次,让平台自动生成 ID
    body=$(curl -sf -X POST "$API/api/devices" -H "$AUTH" -H 'Content-Type: application/json' \
      -d "{\"product_key\":\"$PRODUCT_KEY\",\"name\":\"$id\"}") || {
      echo "   注册 $id 失败" >&2
      exit 1
    }
  }
  local device_id device_secret
  device_id=$(echo "$body" | python3 -c 'import json,sys; print(json.load(sys.stdin)["device_id"])')
  device_secret=$(echo "$body" | python3 -c 'import json,sys; print(json.load(sys.stdin)["device_secret"])')
  printf '{"device_id":"%s","device_secret":"%s"}\n' "$device_id" "$device_secret"
}
export -f register_one

seq 1 "$COUNT" | xargs -P "$CONCURRENCY" -I{} bash -c 'register_one "$@"' _ {} \
  > "$TMP_DIR/creds.ndjson"

python3 - "$TMP_DIR/creds.ndjson" "$CREDS" <<'EOF'
import json, sys
with open(sys.argv[1]) as src:
    rows = [json.loads(line) for line in src if line.strip()]
with open(sys.argv[2], "w") as out:
    json.dump(rows, out, indent=2)
print(f"   已写入 {len(rows)} 条凭据 -> {sys.argv[2]}")
EOF

echo "==> 3/4 启动设备模拟器 (count=$COUNT interval=$INTERVAL broker=$BROKER)"
echo "   Ctrl-C 停止;停止时设备会主动断开并触发平台下线标记。"

if [ "$NO_SIM" -eq 1 ]; then
  echo "   -no-sim 已指定,跳过模拟器启动。凭据文件: $CREDS"
  echo ""
  echo "==> 手动启动命令"
  echo "go run ./cmd/devicesim -stress -count $COUNT -interval $INTERVAL \\"
  echo "  -broker $BROKER -credentials $CREDS \\"
  echo "  -product-key $PRODUCT_KEY -type $DEVICE_TYPE"
  exit 0
fi

echo "==> 4/4 监控提示"
echo "   遥测写入:   curl -s $API/metrics | grep -E 'mqtt|telemetry'"
echo "   平台日志:   docker compose logs -f platform"
echo "   在线设备:   curl -s -H \"$AUTH\" '$API/api/products?page=1&page_size=100' | grep -c online_device_count"

echo ""
echo "==> 开始压测"
go run ./cmd/devicesim \
  -stress -count "$COUNT" -interval "$INTERVAL" \
  -broker "$BROKER" -credentials "$CREDS" \
  -product-key "$PRODUCT_KEY" -type "$DEVICE_TYPE"
