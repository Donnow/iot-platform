export function statusLabel(status) {
  return ({ online: '在线', offline: '离线', inactive: '未激活', deleted: '已删除', active: '处理中', resolved: '已解除', pending: '等待中', success: '成功', failed: '失败', timeout: '超时' })[status] || status || '未知'
}

export function statusClass(status) {
  return `status-${status || 'unknown'}`
}

export function formatTime(value) {
  if (!value) return '暂无记录'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '暂无记录'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

export function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 }).format(value || 0)
}
