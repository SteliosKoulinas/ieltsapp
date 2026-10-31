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

<div class="flex min-h-screen flex-col">
	<header
		class="mx-auto flex w-full max-w-5xl flex-wrap items-center justify-between gap-x-6 gap-y-3 px-5 py-[1.1rem]"
	>
		<a class="text-[1.15rem] font-extrabold tracking-[-0.02em] no-underline" href="/"
			>IELTS Practice</a
		>
		<nav class="flex gap-1.5" aria-label="Main">
			{#each links as l (l.href)}
				<a
					class="rounded-full border-[1.5px] px-[0.85rem] py-[0.4rem] font-semibold no-underline {isCurrent(
						l.href
					)
						? 'border-neutral-900 text-neutral-900 dark:border-neutral-100 dark:text-neutral-100'
						: 'border-transparent text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100'}"
					href={l.href}
					aria-current={isCurrent(l.href) ? 'page' : undefined}>{l.label}</a
				>
			{/each}
		</nav>
	</header>

	<main class="mx-auto w-full max-w-5xl flex-1 px-5 pb-8 pt-2">
		{@render children()}
	</main>

	<footer
		class="mx-auto w-full max-w-5xl border-t border-neutral-300 px-5 pb-8 pt-4 text-sm text-neutral-500 dark:border-neutral-700 dark:text-neutral-400"
	>
		A personal practice tool. Exercises are original practice material, not official IELTS content.
		Results here are not IELTS band scores. AI feedback can contain mistakes.
	</footer>
</div>
