<script setup>
import { toRefs } from 'vue'
import { Activity, Plus, Upload, Workflow } from 'lucide-vue-next'
import { store, openFirmwareModal, openOTAModal } from '../store'
import { formatTime, statusClass, statusLabel } from '../helpers'

const { firmwares, otaTasks } = toRefs(store)

const inProgressDevices = () => otaTasks.value.reduce((total, task) => total + (task.summary?.pending || 0) + (task.summary?.downloading || 0) + (task.summary?.installing || 0), 0)
</script>

<template>
  <section class="content-area">
    <div class="page-intro"><div><span class="eyebrow">RELEASE / OTA</span><h1>固件与 OTA</h1><p>登记固件、创建升级任务并追踪设备阶段。</p></div><div class="page-actions"><button class="ghost-button" @click="openFirmwareModal"><Upload :size="16" />登记固件</button><button class="primary-button" @click="openOTAModal"><Plus :size="17" />创建任务</button></div></div>
    <div class="ota-summary"><article class="metric-card accent-blue"><div class="metric-label"><span>固件版本</span><Upload :size="17" /></div><strong>{{ firmwares.length }}</strong><span class="metric-foot">已登记元数据</span></article><article class="metric-card accent-teal"><div class="metric-label"><span>升级任务</span><Workflow :size="17" /></div><strong>{{ otaTasks.length }}</strong><span class="metric-foot">全部产品任务</span></article><article class="metric-card accent-gold"><div class="metric-label"><span>进行中设备</span><Activity :size="17" /></div><strong>{{ inProgressDevices() }}</strong><span class="metric-foot">等待或执行中</span></article></div>
    <section class="panel table-panel"><div class="panel-heading"><div><span class="panel-kicker">FIRMWARE CATALOG</span><h2>固件元数据</h2></div><span class="toolbar-count">{{ firmwares.length }} 个版本</span></div><div class="table-wrap"><table><thead><tr><th>产品</th><th>版本</th><th>MD5</th><th>文件地址</th><th>登记时间</th></tr></thead><tbody><tr v-for="firmware in firmwares" :key="firmware.id"><td><strong>{{ firmware.product_key }}</strong></td><td>{{ firmware.version }}</td><td><code>{{ firmware.md5 }}</code></td><td class="truncate-cell">{{ firmware.file_url }}</td><td>{{ formatTime(firmware.created_at) }}</td></tr></tbody></table></div><div v-if="!firmwares.length" class="empty-state"><Upload :size="28" /><strong>还没有固件</strong><span>先登记一份固件元数据</span></div></section>
    <section class="panel table-panel ota-table"><div class="panel-heading"><div><span class="panel-kicker">DEPLOYMENT TASKS</span><h2>升级任务</h2></div><span class="toolbar-count">{{ otaTasks.length }} 个任务</span></div><div class="table-wrap"><table><thead><tr><th>任务</th><th>产品 / 版本</th><th>目标设备</th><th>阶段统计</th><th>更新时间</th></tr></thead><tbody><tr v-for="task in otaTasks" :key="task.task_id"><td><strong>{{ task.task_id }}</strong><small>{{ formatTime(task.created_at) }}</small></td><td>{{ task.product_key }} / {{ task.version }}</td><td>{{ task.target_device_ids?.length || 0 }} 台</td><td><span class="stage-summary"><span v-for="stage in ['pending', 'downloading', 'installing', 'success', 'failed']" :key="stage" v-if="task.summary?.[stage]"><i :class="statusClass(stage)"></i>{{ statusLabel(stage) }} {{ task.summary[stage] }}</span></span></td><td>{{ formatTime(task.updated_at) }}</td></tr></tbody></table></div><div v-if="!otaTasks.length" class="empty-state"><Workflow :size="28" /><strong>还没有升级任务</strong><span>创建任务后会在这里追踪进度</span></div></section>
  </section>
</template>
