import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { defineConfig } from 'vite';

// During `npm run dev` the API runs elsewhere (Docker publishes it on
// 127.0.0.1:8081). In production nginx serves the site and proxies /api,
// so the browser always talks to a single origin and no CORS is needed.
const apiTarget = process.env.API_PROXY ?? 'http://127.0.0.1:8081';

export default defineConfig({
	plugins: [
		sveltekit({
			// SvelteKit 3 reads its configuration here, not in svelte.config.js.
			// A single-page app: every route renders in the browser and the web
			// server falls back to 200.html for any URL (see nginx.conf).
			adapter: adapter({ pages: 'build', assets: 'build', fallback: '200.html', strict: false })
		})
	],
	server: { proxy: { '/api': apiTarget } }
});
