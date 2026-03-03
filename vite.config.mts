import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { readFileSync } from 'fs';
import { resolve } from 'path';

const versionJson = JSON.parse(
  readFileSync(resolve('static/version.json'), 'utf-8')
);

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(versionJson.version),
  },
  plugins: [sveltekit()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/files': 'http://localhost:8080',
    },
  },
  ssr: {
    // Ces packages CJS ne sont pas bundlés : Node.js les charge directement.
    external: ['natural', 'pg', 'pdfreader'],
  },
});
