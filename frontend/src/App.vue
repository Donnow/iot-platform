<script setup>
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  Activity,
  BellRing,
  Check,
  ChevronRight,
  CircleHelp,
  Cpu,
  KeyRound,
  LayoutDashboard,
  Menu,
  Package,
  RefreshCw,
  Settings2,
  Upload,
  Workflow,
} from 'lucide-vue-next'
import { store, connectRealtime, disconnectRealtime, loadData, navigate } from './store'
import AuthModal from './components/AuthModal.vue'
import CommandModal from './components/CommandModal.vue'
import DeviceModal from './components/DeviceModal.vue'
import FirmwareModal from './components/FirmwareModal.vue'
import OTAModal from './components/OTAModal.vue'
import ProductModal from './components/ProductModal.vue'
import RuleModal from './components/RuleModal.vue'

const navItems = [
  { id: 'overview', label: '运行总览', icon: LayoutDashboard },
  { id: 'devices', label: '设备管理', icon: Cpu },
  { id: 'alarms', label: '告警中心', icon: BellRing },
  { id: 'rules', label: '规则配置', icon: Workflow },
  { id: 'products', label: '产品与物模型', icon: Package },
  { id: 'ota', label: '固件与 OTA', icon: Upload },
]

const views = {
  overview: defineAsyncComponent(() => import('./views/OverviewView.vue')),
  devices: defineAsyncComponent(() => import('./views/DevicesView.vue')),
  alarms: defineAsyncComponent(() => import('./views/AlarmsView.vue')),
  rules: defineAsyncComponent(() => import('./views/RulesView.vue')),
  products: defineAsyncComponent(() => import('./views/ProductsView.vue')),
  ota: defineAsyncComponent(() => import('./views/OTAView.vue')),
}

const sidebarOpen = ref(false)
const activeAlarms = computed(() => store.alarms.filter((alarm) => alarm.status === 'active').length)
const healthLabel = computed(() => {
  if (store.loading) return '正在同步'
  if (store.connected) return '平台在线'
  return '等待连接'
})
const mqttRealtimeEnabled = computed(() => Boolean(import.meta.env.VITE_MQTT_WS_URL))

watch(() => store.activeView, () => {
  sidebarOpen.value = false
})

onMounted(() => {
  loadData()
  connectRealtime()
})

onBeforeUnmount(() => {
  disconnectRealtime()
})
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar" :class="{ 'sidebar-open': sidebarOpen }">
      <div class="brand-lockup">
        <div class="brand-mark"><Activity :size="20" /></div>
        <div>
          <strong>PERFORM</strong>
          <span>园区设备运维台</span>
        </div>
      </div>
      <nav class="main-nav" aria-label="主导航">
        <button v-for="item in navItems" :key="item.id" class="nav-item" :class="{ active: store.activeView === item.id }" @click="navigate(item.id)">
          <component :is="item.icon" :size="17" />
          <span>{{ item.label }}</span>
          <span v-if="item.id === 'alarms' && activeAlarms" class="nav-count">{{ activeAlarms }}</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="broker-status">
          <span class="status-dot" :class="store.connected ? 'dot-online' : 'dot-idle'"></span>
          <div><strong>{{ healthLabel }}</strong><span>数据服务</span></div>
        </div>
        <button class="nav-item muted" title="系统设置"><Settings2 :size="17" /><span>系统设置</span></button>
      </div>
    </aside>

    <main class="workspace">
      <header class="topbar">
        <button class="icon-button mobile-menu" title="打开导航" @click="sidebarOpen = !sidebarOpen"><Menu :size="19" /></button>
        <div class="breadcrumb"><span>园区运维</span><ChevronRight :size="15" /><strong>{{ navItems.find((item) => item.id === store.activeView)?.label }}</strong></div>
        <div class="topbar-actions">
          <span class="sync-time">{{ store.refreshing ? '正在刷新' : '刚刚同步' }}</span>
          <button class="icon-button" title="刷新数据" :disabled="store.refreshing" @click="loadData({ silent: true })"><RefreshCw :size="17" :class="{ spin: store.refreshing }" /></button>
          <button class="icon-button" title="访问令牌" @click="store.authRequired = true"><KeyRound :size="17" /></button>
          <div class="user-chip"><span class="avatar">OP</span><span>运维账户</span></div>
        </div>
      </header>

      <div v-if="store.errorMessage" class="connection-banner"><CircleHelp :size="17" /><span>{{ store.errorMessage }}</span><button class="text-button" @click="store.authRequired = true">配置连接</button></div>

      <component :is="views[store.activeView] || views.overview" />
    </main>

    <DeviceModal />
    <ProductModal />
    <RuleModal />
    <CommandModal />
    <FirmwareModal />
    <OTAModal />
    <AuthModal />

    <div v-if="store.toastMessage" class="toast"><Check :size="16" />{{ store.toastMessage }}</div>
  </div>
</template>
