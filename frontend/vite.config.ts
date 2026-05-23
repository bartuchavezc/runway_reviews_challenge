import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The API client calls /api/... as relative paths so the same code works in
// dev (via this proxy) and in production (when both apps are served from the
// same origin). PORT defaults to 8080 — matches backend/cmd/server/main.go.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
