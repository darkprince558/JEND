import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  base: './',
  plugins: [react()],
  define: {
    // AWS SDK strictly requires this standard Node global in the browser
    global: 'window',
    'process.env': {}
  }
})
