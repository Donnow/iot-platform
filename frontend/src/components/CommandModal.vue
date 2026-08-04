<script setup>
import { toRefs } from 'vue'
import { Send, X } from 'lucide-vue-next'
import { store, sendCommand } from '../store'

const { showCommandModal, commandDevice, commandForm } = toRefs(store)
</script>

<template>
  <div v-if="showCommandModal" class="modal-backdrop" @click.self="store.showCommandModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">REMOTE COMMAND</span><h2>发送指令</h2></div><button class="icon-button" title="关闭" @click="store.showCommandModal = false"><X :size="18" /></button></div><div class="command-target"><span class="product-icon"><Send :size="17" /></span><div><strong>{{ commandDevice?.name || commandDevice?.device_id }}</strong><small>{{ commandDevice?.device_id }}</small></div></div><div class="form-grid"><label>指令方法<input v-model="commandForm.method" placeholder="open" /></label><label class="span-two">参数 JSON<textarea v-model="commandForm.params" rows="5" spellcheck="false"></textarea></label></div><div class="modal-actions"><button class="ghost-button" @click="store.showCommandModal = false">取消</button><button class="primary-button" @click="sendCommand"><Send :size="16" />立即下发</button></div></section></div>
</template>
