import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = process.env.NEUDRIVE_DEV_API_TARGET || 'http://localhost:8090'
const gatewayProxy = () => ({ target: apiTarget, changeOrigin: true })

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': gatewayProxy(),
      '/agent': gatewayProxy(),
      '/mcp': gatewayProxy(),
      '/gpt': gatewayProxy(),
      '/stripe': gatewayProxy(),
      '/.well-known': gatewayProxy(),
      '/oauth/authorize': {
        ...gatewayProxy(),
        bypass: (req) => {
          if (req.method === 'GET' || req.method === 'HEAD') {
            return '/index.html'
          }
        }
      },
      '/oauth': gatewayProxy()
    }
  }
})
