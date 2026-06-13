// SPDX-License-Identifier: MIT
import { execSync } from 'child_process'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

// Resilient git hash: returns 'nogit' if not in a git repo (fresh scaffold).
function gitShort(): string {
  try {
    return execSync('git rev-parse --short=7 HEAD', { encoding: 'utf8' }).trim()
  } catch {
    return 'nogit'
  }
}
const gitHash = gitShort()
const buildDate = new Date().toISOString().slice(0, 10).replace(/-/g, '.')

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
  define: {
    __APP_VERSION__: JSON.stringify(
      mode === 'production' ? `${buildDate}-${gitHash}` : `dev-${buildDate}-${gitHash}`
    ),
  },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    rollupOptions: {
      output: {
        // Example vendor-chunk splitting. The conditions simply never match for
        // libraries you don't depend on, so this is safe to keep and extend.
        manualChunks(id) {
          if (id.includes('node_modules/recharts') || id.includes('node_modules/d3-') || id.includes('node_modules/victory-vendor')) {
            return 'vendor-charts'
          }
          if (id.includes('node_modules/@radix-ui/')) {
            return 'vendor-radix'
          }
          if (id.includes('node_modules/@tanstack/')) {
            return 'vendor-query'
          }
          if (id.includes('node_modules/react-router') || id.includes('node_modules/react-dom') || (id.includes('node_modules/react/') && !id.includes('react-'))) {
            return 'vendor-react'
          }
        },
      },
    },
  },
  server: {
    proxy: {
      // Example: proxy API calls to a local backend during development.
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
}))
