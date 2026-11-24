import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In dev, the Go server runs on :8025 and Vite proxies the API to it.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: { '/api': { target: process.env.API_URL || 'http://localhost:8025', changeOrigin: true } },
  },
  build: { outDir: 'dist', sourcemap: false },
})
