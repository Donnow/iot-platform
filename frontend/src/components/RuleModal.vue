<script setup>
import { toRefs } from 'vue'
import { Check, X } from 'lucide-vue-next'
import { store, createRule } from '../store'

const { showRuleModal, ruleForm, products } = toRefs(store)
</script>

<template>
  <div v-if="showRuleModal" class="modal-backdrop" @click.self="store.showRuleModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">NEW RULE</span><h2>新建规则</h2></div><button class="icon-button" title="关闭" @click="store.showRuleModal = false"><X :size="18" /></button></div><div class="form-grid"><label>绑定产品<select v-model="ruleForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>规则名称<input v-model="ruleForm.name" placeholder="例如：高温告警" /></label><label>属性名称<input v-model="ruleForm.property_name" placeholder="temperature" /></label><label>比较运算<select v-model="ruleForm.operator"><option v-for="operator in ['>', '<', '>=', '<=', '==', '!=']" :key="operator" :value="operator">{{ operator }}</option></select></label><label>阈值<input v-model="ruleForm.threshold" type="number" /></label><label>持续时间（秒）<input v-model="ruleForm.duration_seconds" type="number" min="0" /></label></div><div class="modal-actions"><button class="ghost-button" @click="store.showRuleModal = false">取消</button><button class="primary-button" @click="createRule"><Check :size="16" />启用规则</button></div></section></div>
</template>
