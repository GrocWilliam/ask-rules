import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    proxy: {
      '/api': 'http://localhost:3001',
      '/files': 'http://localhost:3001',
    },
  },
  ssr: {
    // Ces packages CJS ne sont pas bundlés : Node.js les charge directement.
    external: ['natural', 'pg', 'pdfreader'],
  },
});
