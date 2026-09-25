import { defineConfig } from 'vite';

export default defineConfig({
  base: '/qnext/admin/',
  build: {
    outDir: 'dist',
    sourcemap: true,
    emptyOutDir: true,
  },
});
