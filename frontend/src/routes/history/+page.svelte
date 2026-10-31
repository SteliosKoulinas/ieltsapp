<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type AttemptSummary } from '#lib/api';
	import { typeLabel } from '#lib/labels';
	import { formatDateTime } from '#lib/format';
	import { alert, alertTitle, btn, btnPrimary, card, muted, narrow, small } from '#lib/ui';
	import VerdictBadge from '#lib/components/VerdictBadge.svelte';

	const PAGE = 20;

	let items = $state<AttemptSummary[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state<string | null>(null);

	async function load(more: boolean) {
		loading = true;
		error = null;
		try {
			const res = await api.listAttempts(PAGE, more ? items.length : 0);
			items = more ? [...items, ...res.items] : res.items;
			total = res.total;
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not load your history.';
		} finally {
			loading = false;
		}
	}

	onMount(() => load(false));

	function headline(a: AttemptSummary): string {
		return a.type === 'vocab' && a.target_word ? `Word: ${a.target_word}` : a.prompt;
	}
</script>

<svelte:head><title>History · IELTS Practice</title></svelte:head>

<div class={narrow}>
	<h1 class="mb-2 text-[1.75rem] font-bold tracking-[-0.01em]">History</h1>
	<p class={muted}>Every answer you submitted, newest first. Open one to see the full feedback.</p>

	{#if error}
		<div class={alert} role="alert">
			<strong class={alertTitle}>Problem</strong><span>{error}</span>
		</div>
		<button class={btn} type="button" onclick={() => load(false)}>Try again</button>
	{/if}

	{#if loading && items.length === 0}
		<div class={card} aria-busy="true"><p class={muted}>Loading…</p></div>
	{:else if !error && items.length === 0}
		<div class={card}>
			<p>You have not answered anything yet.</p>
			<a class={btnPrimary} href="/">Start practising</a>
		</div>
	{:else}
		<ul class="mb-4 list-none p-0">
			{#each items as a (a.id)}
				<li>
					<a
						class="mb-[0.6rem] flex flex-col gap-[0.15rem] rounded-[10px] border border-neutral-300 bg-neutral-50 px-4 py-[0.9rem] no-underline hover:border-neutral-900 dark:border-neutral-700 dark:bg-neutral-900 dark:hover:border-neutral-100"
						href="/history/{a.id}"
					>
						<span class="flex items-center justify-between gap-3">
							<span class={`${small} ${muted}`}>{formatDateTime(a.created_at)}</span>
							{#if a.status === 'evaluated' && a.verdict}
								<VerdictBadge verdict={a.verdict} />
							{:else}
								<span
									class={`${small} rounded-full border-[1.5px] border-dashed border-neutral-900 px-[0.65rem] py-[0.15rem] font-semibold dark:border-neutral-100`}
									>Saved, not evaluated</span
								>
							{/if}
						</span>
						<span class="font-semibold">{headline(a)}</span>
						<span class={`${small} ${muted} line-clamp-2 overflow-hidden break-words`}
							>{a.answer}</span
						>
						<span class={`${small} ${muted}`}>
							{typeLabel[a.type]}{#if a.is_rewrite} · Rewrite{/if}
						</span>
					</a>
				</li>
			{/each}
		</ul>

		{#if items.length < total}
			<button class={btn} type="button" disabled={loading} onclick={() => load(true)}>
				{loading ? 'Loading…' : `Show more (${total - items.length} left)`}
			</button>
		{/if}
	{/if}
</div>
