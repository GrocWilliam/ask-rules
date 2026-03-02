import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
export default {
  kit: {
    adapter: adapter({
      pages: 'server/build',
      assets: 'server/build',
      fallback: 'index.html',
    }),
  },
};
