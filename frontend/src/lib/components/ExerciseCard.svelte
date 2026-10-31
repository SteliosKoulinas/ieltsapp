<script lang="ts">
	import type { Exercise } from '#lib/api';
	import { typeLabel } from '#lib/labels';
	import { card, muted, small } from '#lib/ui';

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

<section class={card} aria-labelledby="exercise-heading">
	<p class={`${small} ${muted} mb-3 flex flex-wrap gap-[0.4rem]`}>
		{#if exercise.mode}
			<span class="font-bold text-neutral-900 dark:text-neutral-100"
				>{exercise.mode === 'review' ? 'Review' : 'New word'}</span
			>
			<span aria-hidden="true">·</span>
		{/if}
		<span>{typeLabel[exercise.type]}</span>
		<span aria-hidden="true">·</span>
		<span>{exercise.topic}</span>
		<span aria-hidden="true">·</span>
		<span title="Approximate level, not an official CEFR rating">~{exercise.cefr}</span>
	</p>

	{#if exercise.type === 'vocab'}
		<h2 id="exercise-heading" class="sr-only">Vocabulary exercise</h2>
		<blockquote
			class="mb-[0.9rem] border-l-[3px] border-neutral-900 pl-4 text-[1.15rem] leading-[1.55] dark:border-neutral-100"
		>
			{#each parts as part, i (i)}{#if part.hit}<mark
						class="rounded-[4px] bg-neutral-900 px-[0.35em] py-[0.05em] font-bold text-white dark:bg-neutral-100 dark:text-neutral-900"
						>{part.text}</mark
					>{:else}{part.text}{/if}{/each}
		</blockquote>
		<h2 id="exercise-heading" class="mb-[0.4rem] text-[1.1rem] font-semibold">
			{exercise.prompt}
		</h2>
	{:else}
		<h2 id="exercise-heading" class="mb-[0.4rem] text-[1.1rem] font-semibold">
			{exercise.prompt}
		</h2>
		{#if exercise.expected_length}
			<p class={`${small} ${muted}`}>Write about {exercise.expected_length}.</p>
		{/if}
		{#if exercise.criteria?.length}
			<p class={`${small} ${muted}`}>We look at: {exercise.criteria.join(', ')}.</p>
		{/if}
	{/if}
</section>
