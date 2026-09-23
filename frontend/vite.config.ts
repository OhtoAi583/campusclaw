import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发环境与生产环境保持同一条路径：前端只调用同源 /api，
// 由 Vite 或 Nginx 反代到 Go 后端。这样 Cookie 不跨站，SameSite=Lax 足够。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: false },
      '/health': { target: 'http://localhost:8080', changeOrigin: false },
    },
  },
})
