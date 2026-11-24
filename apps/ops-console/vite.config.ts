import { defineConfig } from 'vite';

export default defineConfig({
  base: '/qnext/admin/',
  build: {
    outDir: 'dist',
    sourcemap: false,
    emptyOutDir: true,
  },
});
