#!/bin/bash
# 本地签发 HS256 JWT，用于开发环境调用平台 API。
# 平台 API 使用 JWT 认证（见 internal/platform/httpapi/jwt.go），
# 签名密钥为 .env 中的 IOT_JWT_SECRET。
#
# 用法：
#   ./scripts/make-jwt.sh                 # 读取环境变量 IOT_JWT_SECRET
#   ./scripts/make-jwt.sh "<secret>"      # 或直接传密钥
#   TOKEN=$(IOT_JWT_SECRET=xxx ./scripts/make-jwt.sh)
#
# 输出一行 token，默认 1 小时过期，可通过第二个参数覆盖：
#   ./scripts/make-jwt.sh "$SECRET" 7200  # 2 小时
set -euo pipefail

SECRET="${1:-${IOT_JWT_SECRET:-}}"
TTL="${2:-3600}"

if [ -z "$SECRET" ]; then
  echo "用法: IOT_JWT_SECRET=<secret> ./scripts/make-jwt.sh [ttl_seconds]" >&2
  echo "     或 ./scripts/make-jwt.sh <secret> [ttl_seconds]" >&2
  exit 1
fi

b64url() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

header=$(printf '{"alg":"HS256","typ":"JWT"}' | b64url)
payload=$(printf '{"exp":%d}' "$(( $(date +%s) + TTL ))" | b64url)
signature=$(printf '%s.%s' "$header" "$payload" | openssl dgst -sha256 -hmac "$SECRET" -binary | b64url)

echo "$header.$payload.$signature"
