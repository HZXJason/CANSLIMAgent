import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined;
          if (id.includes('/recharts/') || id.includes('/d3-')) return 'vendor-charts';
          if (
            id.includes('/react-markdown/') ||
            id.includes('/remark-gfm/') ||
            id.includes('/rehype-sanitize/') ||
            id.includes('/unified/') ||
            id.includes('/micromark') ||
            id.includes('/mdast') ||
            id.includes('/hast') ||
            id.includes('/unist')
          ) {
            return 'vendor-markdown';
          }
          if (id.includes('/@tanstack/react-query/')) return 'vendor-query';
          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/react-router-dom/')) return 'vendor-react';
          return undefined;
        }
      }
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080'
    }
  }
});
