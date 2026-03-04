import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
export default {
  kit: {
    adapter: adapter({
      pages: 'server/cmd/server/build',
      assets: 'server/cmd/server/build',
      fallback: 'index.html',
    }),
  },
};
