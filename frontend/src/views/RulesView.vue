<script setup>
import { toRefs } from 'vue'
import { ChevronRight, Plus, SlidersHorizontal } from 'lucide-vue-next'
import { store } from '../store'

const { rules } = toRefs(store)
</script>

<template>
  <section class="content-area">
    <div class="page-intro"><div><span class="eyebrow">AUTOMATION / RULES</span><h1>规则配置</h1><p>为产品定义实时阈值和告警策略。</p></div><button class="primary-button" @click="store.showRuleModal = true"><Plus :size="17" />新建规则</button></div>
    <section class="panel table-panel"><div class="table-wrap"><table><thead><tr><th>规则</th><th>产品</th><th>条件</th><th>持续时间</th><th>动作</th><th>状态</th></tr></thead><tbody><tr v-for="rule in rules" :key="rule.id"><td><strong>{{ rule.name }}</strong><small>{{ rule.id }}</small></td><td>{{ rule.product_key }}</td><td class="condition-cell"><span>{{ rule.property_name }}</span><b>{{ rule.operator }}</b><span>{{ rule.threshold }}</span></td><td>{{ rule.duration_seconds || 0 }} 秒</td><td>{{ rule.action_type === 'alarm' ? '生成告警' : '下发指令' }}</td><td><span class="status-pill" :class="rule.enabled ? 'status-online' : 'status-inactive'"><i></i>{{ rule.enabled ? '已启用' : '已停用' }}</span></td></tr></tbody></table></div><div v-if="!rules.length" class="empty-state"><SlidersHorizontal :size="30" /><strong>还没有规则</strong><span>创建第一条阈值策略</span><button class="text-button" @click="store.showRuleModal = true">新建规则 <ChevronRight :size="14" /></button></div></section>
  </section>
</template>
