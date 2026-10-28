<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';

	let { children } = $props();

	const links = [
		{ href: '/', label: 'Practice' },
		{ href: '/history', label: 'History' },
		{ href: '/stats', label: 'Statistics' }
	];

	function isCurrent(href: string): boolean {
		const p = page.url.pathname;
		return href === '/' ? p === '/' : p === href || p.startsWith(href + '/');
	}
</script>

<div class="shell">
	<header>
		<a class="brand" href="/">IELTS Practice</a>
		<nav aria-label="Main">
			{#each links as l (l.href)}
				<a href={l.href} aria-current={isCurrent(l.href) ? 'page' : undefined}>{l.label}</a>
			{/each}
		</nav>
	</header>

	<main>
		{@render children()}
	</main>

	<footer class="small muted">
		A personal practice tool. Exercises are original practice material, not official IELTS content.
		Results here are not IELTS band scores. AI feedback can contain mistakes.
	</footer>
</div>

<style>
	.shell {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}
	header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem 1.5rem;
		max-width: var(--content-wide);
		width: 100%;
		margin: 0 auto;
		padding: 1.1rem 1.25rem;
	}
	.brand {
		font-weight: 800;
		font-size: 1.15rem;
		letter-spacing: -0.02em;
		text-decoration: none;
	}
	nav {
		display: flex;
		gap: 0.35rem;
	}
	nav a {
		text-decoration: none;
		font-weight: 600;
		padding: 0.4rem 0.85rem;
		border-radius: 999px;
		border: 1.5px solid transparent;
		color: var(--muted);
	}
	nav a:hover {
		color: var(--text);
	}
	nav a[aria-current='page'] {
		color: var(--text);
		border-color: var(--strong);
	}
	main {
		flex: 1;
		width: 100%;
		max-width: var(--content-wide);
		margin: 0 auto;
		padding: 0.5rem 1.25rem 2rem;
	}
	footer {
		max-width: var(--content-wide);
		width: 100%;
		margin: 0 auto;
		padding: 1rem 1.25rem 2rem;
		border-top: 1px solid var(--border);
	}
</style>
