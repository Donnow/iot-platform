<script setup>
import { toRefs } from 'vue'
import { Upload, X } from 'lucide-vue-next'
import { store, createFirmware } from '../store'

const { showFirmwareModal, firmwareForm, products } = toRefs(store)
</script>

<template>
  <div v-if="showFirmwareModal" class="modal-backdrop" @click.self="store.showFirmwareModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">FIRMWARE CATALOG</span><h2>登记固件</h2></div><button class="icon-button" title="关闭" @click="store.showFirmwareModal = false"><X :size="18" /></button></div><div class="form-grid"><label>所属产品<select v-model="firmwareForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>版本号<input v-model="firmwareForm.version" placeholder="1.2.3" /></label><label class="span-two">MD5<input v-model="firmwareForm.md5" placeholder="32 位十六进制摘要" /></label><label class="span-two">文件地址<input v-model="firmwareForm.file_url" placeholder="https://.../firmware.bin" /></label><label class="span-two">变更说明<textarea v-model="firmwareForm.changelog" rows="3"></textarea></label></div><div class="modal-actions"><button class="ghost-button" @click="store.showFirmwareModal = false">取消</button><button class="primary-button" @click="createFirmware"><Upload :size="16" />登记固件</button></div></section></div>
</template>
