import { defineConfig } from 'vite';

export default defineConfig({
  base: '/qnext/',
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:18080',
        changeOrigin: true,
        ws: true,
      },
      '/health': {
        target: 'http://127.0.0.1:18080',
        changeOrigin: true,
      },
      '/ready': {
        target: 'http://127.0.0.1:18080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
    emptyOutDir: true,
  },
});
