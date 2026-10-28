<script lang="ts">
	import type { Exercise } from '#lib/api';
	import { typeLabel } from '#lib/labels';

	let { exercise }: { exercise: Exercise } = $props();

	function escapeRegExp(s: string): string {
		return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	}

	// Split the context sentence so the target word can be highlighted.
	const parts = $derived.by(() => {
		const ctx = exercise.context ?? '';
		const word = exercise.target_word ?? '';
		if (!ctx || !word) return [{ text: ctx, hit: false }];
		const re = new RegExp(`(${escapeRegExp(word)})`, 'gi');
		return ctx.split(re).map((text, i) => ({ text, hit: i % 2 === 1 }));
	});
</script>

<section class="card exercise" aria-labelledby="exercise-heading">
	<p class="meta small muted">
		{#if exercise.mode}
			<span class="mode">{exercise.mode === 'review' ? 'Review' : 'New word'}</span>
			<span aria-hidden="true">·</span>
		{/if}
		<span>{typeLabel[exercise.type]}</span>
		<span aria-hidden="true">·</span>
		<span>{exercise.topic}</span>
		<span aria-hidden="true">·</span>
		<span title="Approximate level, not an official CEFR rating">~{exercise.cefr}</span>
	</p>

	{#if exercise.type === 'vocab'}
		<h2 id="exercise-heading" class="visually-hidden">Vocabulary exercise</h2>
		<blockquote class="context">
			{#each parts as part, i (i)}{#if part.hit}<mark>{part.text}</mark>{:else}{part.text}{/if}{/each}
		</blockquote>
		<p class="prompt">{exercise.prompt}</p>
	{:else}
		<h2 id="exercise-heading" class="prompt">{exercise.prompt}</h2>
		{#if exercise.expected_length}
			<p class="small muted">Write about {exercise.expected_length}.</p>
		{/if}
		{#if exercise.criteria?.length}
			<p class="small muted">We look at: {exercise.criteria.join(', ')}.</p>
		{/if}
	{/if}
</section>

<style>
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin-bottom: 0.75rem;
	}
	.mode {
		font-weight: 700;
		color: var(--text);
	}
	.context {
		margin: 0 0 0.9rem;
		padding: 0 0 0 1rem;
		border-left: 3px solid var(--strong);
		font-size: 1.15rem;
		line-height: 1.55;
	}
	mark {
		background: var(--strong);
		color: var(--on-strong);
		padding: 0.05em 0.35em;
		border-radius: 4px;
		font-weight: 700;
	}
	.prompt {
		font-size: 1.1rem;
		font-weight: 650;
		margin-bottom: 0.4rem;
	}
</style>
