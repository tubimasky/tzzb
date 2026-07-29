import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // echarts 体积远大于业务代码，单独成块：改页面不会让图表库缓存失效。
    // 阈值调到 700kB，避免为已知的 echarts 块反复告警。
    chunkSizeWarningLimit: 700,
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: 'echarts', test: /node_modules[\\/](echarts|zrender)[\\/]/ },
            { name: 'react', test: /node_modules[\\/](react|react-dom|react-router|react-router-dom)[\\/]/ },
          ],
        },
      },
    },
  },
})
