// 构建产物 bundle budget 检查：任何 JS chunk 超过预算即失败，并打印体积排行。
// 预算与 vite.config.js 的 build.chunkSizeWarningLimit 保持一致（500 kB）。
import { readdirSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const budgetKb = 500
const assetsDir = join(dirname(fileURLToPath(import.meta.url)), '../dist/assets')
const files = readdirSync(assetsDir).filter((name) => name.endsWith('.js'))
const chunks = files
  .map((name) => ({ name, kb: Math.round(statSync(join(assetsDir, name)).size / 1024) }))
  .sort((a, b) => b.kb - a.kb)

let failed = false
for (const chunk of chunks) {
  const marker = chunk.kb > budgetKb ? 'FAIL' : '  ok'
  console.log(`[bundle] ${marker} ${String(chunk.kb).padStart(5)} kB  ${chunk.name}`)
  if (chunk.kb > budgetKb) failed = true
}
if (failed) {
  console.error(`[bundle] 存在超过 ${budgetKb} kB 的 chunk，超出 bundle budget`)
  process.exit(1)
}
console.log(`[bundle] 全部 ${chunks.length} 个 chunk 均在 ${budgetKb} kB 预算内`)
