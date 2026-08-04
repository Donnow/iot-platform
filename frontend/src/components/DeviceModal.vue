<script setup>
import { toRefs } from 'vue'
import { Check, KeyRound, Plus, X } from 'lucide-vue-next'
import { store, createDevice } from '../store'

const { showDeviceModal, revealedSecret, deviceForm, products } = toRefs(store)
</script>

<template>
  <div v-if="showDeviceModal" class="modal-backdrop" @click.self="store.showDeviceModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">NEW DEVICE</span><h2>注册设备</h2></div><button class="icon-button" title="关闭" @click="store.showDeviceModal = false"><X :size="18" /></button></div><div v-if="revealedSecret" class="secret-callout"><KeyRound :size="18" /><div><strong>设备密钥只显示这一次</strong><code>{{ revealedSecret }}</code></div><button class="icon-button small" title="关闭密钥提示" @click="store.showDeviceModal = false"><Check :size="16" /></button></div><div class="form-grid"><label>设备名称<input v-model="deviceForm.name" placeholder="例如：一号楼温湿度 01" /></label><label>所属产品<select v-model="deviceForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>设备 ID（可选）<input v-model="deviceForm.device_id" placeholder="系统自动生成" /></label><label>描述（可选）<input v-model="deviceForm.description" placeholder="位置或用途" /></label></div><div class="modal-actions"><button class="ghost-button" @click="store.showDeviceModal = false">取消</button><button class="primary-button" @click="createDevice"><Plus :size="16" />创建设备</button></div></section></div>
</template>
