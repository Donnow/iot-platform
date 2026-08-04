<script setup>
import { computed, ref, toRefs } from 'vue'
import { Check, Cpu, Plus, RefreshCw, Search, Send, Trash2 } from 'lucide-vue-next'
import { store, deleteDevice, loadShadow, openCommand, openDeviceModal, saveDesired, selectDevice } from '../store'
import { formatTime, statusClass, statusLabel } from '../helpers'

const { devices, deviceShadow, desiredInput, selectedDeviceId, commandStatus, commandPolling } = toRefs(store)

const deviceSearch = ref('')
const deviceStatusFilter = ref('')

const filteredDevices = computed(() => devices.value.filter((device) => {
  const matchesSearch = !deviceSearch.value || `${device.name} ${device.device_id} ${device.product_key}`.toLowerCase().includes(deviceSearch.value.toLowerCase())
  const matchesStatus = !deviceStatusFilter.value || device.status === deviceStatusFilter.value
  return matchesSearch && matchesStatus
}))
const selectedDevice = computed(() => devices.value.find((device) => device.device_id === selectedDeviceId.value) || null)
</script>

<template>
  <section class="content-area">
    <div class="page-intro"><div><span class="eyebrow">ASSET / DEVICES</span><h1>设备管理</h1><p>管理设备身份、连接状态、影子和远程操作。</p></div><button class="primary-button" @click="openDeviceModal"><Plus :size="17" />注册设备</button></div>
    <div class="toolbar"><div class="search-field"><Search :size="16" /><input v-model="deviceSearch" placeholder="搜索设备名称、ID 或产品" /></div><select v-model="deviceStatusFilter" class="filter-select"><option value="">全部状态</option><option value="online">在线</option><option value="offline">离线</option><option value="inactive">未激活</option></select><span class="toolbar-count">{{ filteredDevices.length }} 台设备</span></div>
    <div class="split-layout"><section class="panel table-panel"><div class="table-wrap"><table><thead><tr><th>设备</th><th>产品</th><th>状态</th><th>最后在线</th><th class="align-right">操作</th></tr></thead><tbody><tr v-for="device in filteredDevices" :key="device.device_id" :class="{ selected: selectedDeviceId === device.device_id }" @click="selectDevice(device.device_id)"><td><strong>{{ device.name || device.device_id }}</strong><small>{{ device.device_id }}</small></td><td>{{ device.product_key }}</td><td><span class="status-pill" :class="statusClass(device.status)"><i></i>{{ statusLabel(device.status) }}</span></td><td>{{ formatTime(device.last_online) }}</td><td class="align-right"><button class="icon-button small" title="发送指令" @click.stop="openCommand(device)"><Send :size="15" /></button><button class="icon-button small danger-icon" title="删除设备" @click.stop="deleteDevice(device)"><Trash2 :size="15" /></button></td></tr></tbody></table></div><div v-if="!filteredDevices.length" class="empty-state"><Search :size="28" /><strong>没有匹配设备</strong><span>调整筛选条件后再试</span></div></section><aside class="panel device-detail"><div class="panel-heading"><div><span class="panel-kicker">SELECTED DEVICE</span><h2>{{ selectedDevice?.name || '设备详情' }}</h2></div><span v-if="selectedDevice" class="status-pill" :class="statusClass(selectedDevice.status)"><i></i>{{ statusLabel(selectedDevice.status) }}</span></div><div v-if="selectedDevice" class="detail-content"><dl><div><dt>设备 ID</dt><dd>{{ selectedDevice.device_id }}</dd></div><div><dt>产品</dt><dd>{{ selectedDevice.product_key }}</dd></div><div><dt>创建时间</dt><dd>{{ formatTime(selectedDevice.created_at) }}</dd></div></dl><div class="detail-actions detail-actions-grid"><button class="primary-button" @click="openCommand(selectedDevice)"><Send :size="16" />发送指令</button><button class="ghost-button danger-button" @click="deleteDevice(selectedDevice)"><Trash2 :size="15" />删除设备</button></div><div class="state-block"><div class="state-heading"><div><span class="panel-kicker">DEVICE SHADOW</span><strong>影子状态</strong></div><button class="icon-button small" title="刷新影子" @click="loadShadow(selectedDevice.device_id)"><RefreshCw :size="14" /></button></div><textarea v-model="desiredInput" rows="4" spellcheck="false" aria-label="期望状态 JSON"></textarea><button class="ghost-button full" @click="saveDesired(selectedDevice.device_id)"><Check :size="14" />保存 desired</button><div v-if="deviceShadow" class="shadow-values"><span><b>reported</b><code>{{ JSON.stringify(deviceShadow.reported || {}) }}</code></span><span><b>delta</b><code>{{ JSON.stringify(deviceShadow.delta || {}) }}</code></span></div></div><div v-if="commandStatus" class="command-status"><span class="panel-kicker">LAST COMMAND</span><strong>{{ commandStatus.method }} · {{ statusLabel(commandStatus.status) }}</strong><small>{{ commandStatus.message || (commandPolling ? '等待设备回复' : '暂无回复') }}</small></div></div><div v-else class="empty-state compact"><Cpu :size="25" /><strong>选择一台设备</strong><span>详情、影子和操作会在这里展开</span></div></aside></div>
  </section>
</template>
