<script setup>
import { toRefs } from 'vue'
import { Package, Plus } from 'lucide-vue-next'
import { store, openProductModal } from '../store'

const { products, devices, rules } = toRefs(store)
</script>

<template>
  <section class="content-area">
    <div class="page-intro"><div><span class="eyebrow">CATALOG / MODELS</span><h1>产品与物模型</h1><p>产品定义设备类型和遥测属性边界。</p></div><button class="primary-button" @click="openProductModal"><Plus :size="17" />新建产品</button></div>
    <div class="product-grid"><article v-for="product in products" :key="product.product_key" class="product-card"><div class="product-card-top"><span class="product-icon"><Package :size="18" /></span><span class="status-pill status-online"><i></i>已接入</span></div><h2>{{ product.name }}</h2><code>{{ product.product_key }}</code><p>{{ product.description || '暂无产品描述' }}</p><div class="property-list"><span v-for="property in (product.properties || []).slice(0, 4)" :key="property.name">{{ property.name }} <small>{{ property.data_type }}</small></span><span v-if="!product.properties?.length" class="muted-text">未定义属性</span></div><div class="product-card-foot"><span>{{ devices.filter((device) => device.product_key === product.product_key).length }} 台设备</span><span>{{ rules.filter((rule) => rule.product_key === product.product_key).length }} 条规则</span></div></article></div><div v-if="!products.length" class="empty-state panel"><Package :size="30" /><strong>还没有产品</strong><span>先创建产品，再注册设备并定义数据边界</span><button class="primary-button" @click="openProductModal"><Plus :size="16" />新建产品</button></div>
  </section>
</template>
