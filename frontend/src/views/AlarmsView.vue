<script setup>
import { computed, ref, toRefs } from 'vue'
import { Check, ShieldCheck } from 'lucide-vue-next'
import { store, resolveAlarm } from '../store'
import { formatNumber, formatTime, statusClass, statusLabel } from '../helpers'

const { alarms } = toRefs(store)
const alarmStatusFilter = ref('')

const activeAlarms = computed(() => alarms.value.filter((alarm) => alarm.status === 'active').length)
const filteredAlarms = computed(() => alarms.value.filter((alarm) => !alarmStatusFilter.value || alarm.status === alarmStatusFilter.value))
</script>

<template>
  <section class="content-area">
    <div class="page-intro"><div><span class="eyebrow">ATTENTION / ALERTS</span><h1>告警中心</h1><p>跟踪异常状态并留下处理结果。</p></div><div class="page-stat"><strong>{{ activeAlarms }}</strong><span>活跃告警</span></div></div>
    <div class="toolbar"><div class="filter-tabs"><button :class="{ active: alarmStatusFilter === '' }" @click="alarmStatusFilter = ''">全部</button><button :class="{ active: alarmStatusFilter === 'active' }" @click="alarmStatusFilter = 'active'">处理中</button><button :class="{ active: alarmStatusFilter === 'resolved' }" @click="alarmStatusFilter = 'resolved'">已解除</button></div><span class="toolbar-count">{{ filteredAlarms.length }} 条记录</span></div>
    <section class="panel table-panel"><div class="table-wrap"><table><thead><tr><th>状态</th><th>设备</th><th>规则</th><th>触发值</th><th>触发时间</th><th class="align-right">处理</th></tr></thead><tbody><tr v-for="alarm in filteredAlarms" :key="alarm.id"><td><span class="status-pill" :class="statusClass(alarm.status)"><i></i>{{ statusLabel(alarm.status) }}</span></td><td><strong>{{ alarm.device_id }}</strong></td><td>{{ alarm.rule_id || '规则' }}</td><td class="value-cell">{{ formatNumber(alarm.trigger_value) }}</td><td>{{ formatTime(alarm.triggered_at) }}</td><td class="align-right"><button v-if="alarm.status === 'active'" class="ghost-button compact-action" @click="resolveAlarm(alarm)"><Check :size="14" />解除</button><span v-else class="muted-text">{{ formatTime(alarm.resolved_at) }}</span></td></tr></tbody></table></div><div v-if="!filteredAlarms.length" class="empty-state"><ShieldCheck :size="30" /><strong>没有告警记录</strong><span>规则命中后会在这里留下记录</span></div></section>
  </section>
</template>
