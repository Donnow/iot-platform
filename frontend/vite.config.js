import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
      '/metrics': 'http://localhost:8080',
      '/internal': 'http://localhost:8080',
    },
  },
  preview: {
    port: 4173,
  },
  test: {
    environment: 'jsdom',
  },
})
