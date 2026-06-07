import { existsSync } from 'node:fs';
import react from '@vitejs/plugin-react';
import type { PluginOption } from 'vite';
import { defineConfig } from 'vitest/config';

const emptyMainEntryFallback = (): PluginOption => ({
  name: 'empty-main-entry-fallback',
  resolveId(id) {
    if (id === '/src/main.tsx' && !existsSync(new URL('./src/main.tsx', import.meta.url))) {
      return '\0empty-main-entry';
    }
    return null;
  },
  load(id) {
    if (id === '\0empty-main-entry') {
      return '';
    }
    return null;
  },
});

export default defineConfig({
  plugins: [emptyMainEntryFallback(), react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './vitest.setup.ts',
    globals: true,
  },
});
