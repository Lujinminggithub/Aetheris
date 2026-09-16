import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export const ADMIN_BASE_PATH = '/admin/'

export default defineConfig({
  base: ADMIN_BASE_PATH,
  plugins: [react()],
  server: {
    proxy: {
      '/api': { target: process.env.AETHERIS_API_URL || 'http://127.0.0.1:8080', changeOrigin: true },
      '/admin': { target: process.env.AETHERIS_API_URL || 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
})
