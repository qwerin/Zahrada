import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Vývoj: `make dev-backend` (:8080) + `make dev-frontend` (:5173, /api se proxuje na backend)
export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/api': 'http://localhost:8080' } },
})
