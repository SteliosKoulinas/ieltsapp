<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type AttemptSummary } from '#lib/api';
	import { typeLabel } from '#lib/labels';
	import { formatDateTime } from '#lib/format';
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

<div class="narrow">
	<h1>History</h1>
	<p class="muted">Every answer you submitted, newest first. Open one to see the full feedback.</p>

	{#if error}
		<div class="alert" role="alert"><strong>Problem</strong><span>{error}</span></div>
		<button class="btn" type="button" onclick={() => load(false)}>Try again</button>
	{/if}

	{#if loading && items.length === 0}
		<div class="card" aria-busy="true"><p class="muted">Loading…</p></div>
	{:else if !error && items.length === 0}
		<div class="card">
			<p>You have not answered anything yet.</p>
			<a class="btn primary" href="/">Start practising</a>
		</div>
	{:else}
		<ul class="list">
			{#each items as a (a.id)}
				<li>
					<a class="item" href="/history/{a.id}">
						<span class="line1">
							<span class="when small muted">{formatDateTime(a.created_at)}</span>
							{#if a.status === 'evaluated' && a.verdict}
								<VerdictBadge verdict={a.verdict} />
							{:else}
								<span class="pending small">Saved, not evaluated</span>
							{/if}
						</span>
						<span class="title">{headline(a)}</span>
						<span class="answer small muted">{a.answer}</span>
						<span class="tags small muted">
							{typeLabel[a.type]}{#if a.is_rewrite} · Rewrite{/if}
						</span>
					</a>
				</li>
			{/each}
		</ul>

		{#if items.length < total}
			<button class="btn" type="button" disabled={loading} onclick={() => load(true)}>
				{loading ? 'Loading…' : `Show more (${total - items.length} left)`}
			</button>
		{/if}
	{/if}
</div>

<style>
	.narrow {
		max-width: var(--content);
	}
	.list {
		list-style: none;
		padding: 0;
		margin: 0 0 1rem;
	}
	.item {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		padding: 0.9rem 1rem;
		margin-bottom: 0.6rem;
		border: 1px solid var(--border);
		border-radius: var(--radius);
		text-decoration: none;
		background: var(--surface);
	}
	.item:hover {
		border-color: var(--strong);
	}
	.line1 {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.75rem;
	}
	.title {
		font-weight: 650;
	}
	.answer {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		overflow-wrap: anywhere;
	}
	.pending {
		font-weight: 650;
		border: 1.5px dashed var(--strong);
		border-radius: 999px;
		padding: 0.15rem 0.65rem;
	}
</style>
