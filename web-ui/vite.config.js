import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  base: '/panel/assets/',
  build: {
    assetsDir: '', // Keeps assets in the root of the dist directory for simple Go server mapping
  }
})
