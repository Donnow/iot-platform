<script setup>
import { toRefs } from 'vue'
import { KeyRound, ShieldCheck } from 'lucide-vue-next'
import { store, clearToken, submitLogin } from '../store'
import { getAuthToken } from '../api'

const { authRequired, loginUsername, loginPassword, submitting } = toRefs(store)
</script>

<template>
  <div v-if="authRequired" class="modal-backdrop"><section class="modal auth-modal"><div class="auth-mark"><ShieldCheck :size="23" /></div><span class="eyebrow">SECURE ACCESS</span><h2>登录平台</h2><p>使用平台管理员账号登录，获取访问令牌。</p><label>用户名<input v-model="loginUsername" type="text" placeholder="admin" autocomplete="username" @keyup.enter="submitLogin" /></label><label>密码<input v-model="loginPassword" type="password" placeholder="••••••••" autocomplete="current-password" @keyup.enter="submitLogin" /></label><div class="modal-actions"><button v-if="getAuthToken()" class="ghost-button" @click="clearToken">退出登录</button><button class="primary-button" :disabled="submitting" @click="submitLogin"><KeyRound :size="16" />{{ submitting ? '登录中…' : '登录' }}</button></div></section></div>
</template>
