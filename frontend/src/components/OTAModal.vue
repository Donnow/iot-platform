<script setup>
import { toRefs } from 'vue'
import { Upload, X } from 'lucide-vue-next'
import { store, createOTATask } from '../store'

const { showOTAModal, otaForm, products, firmwares } = toRefs(store)
</script>

<template>
  <div v-if="showOTAModal" class="modal-backdrop" @click.self="store.showOTAModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">OTA DEPLOYMENT</span><h2>创建升级任务</h2></div><button class="icon-button" title="关闭" @click="store.showOTAModal = false"><X :size="18" /></button></div><div class="form-grid"><label>所属产品<select v-model="otaForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>固件版本<select v-model="otaForm.firmware_id"><option value="" disabled>选择固件</option><option v-for="firmware in firmwares.filter((item) => item.product_key === otaForm.product_key)" :key="firmware.id" :value="firmware.id">{{ firmware.version }}</option></select></label><label>目标范围<select v-model="otaForm.target"><option value="all">产品下全部设备</option><option value="devices">指定设备</option></select></label><label v-if="otaForm.target === 'devices'" class="span-two">设备 ID（可用空格或逗号分隔）<textarea v-model="otaForm.target_device_ids" rows="3" placeholder="temp-001 temp-002"></textarea></label></div><div class="modal-actions"><button class="ghost-button" @click="store.showOTAModal = false">取消</button><button class="primary-button" @click="createOTATask"><Upload :size="16" />创建任务</button></div></section></div>
</template>
