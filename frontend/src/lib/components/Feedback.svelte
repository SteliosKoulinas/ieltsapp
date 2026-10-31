<script lang="ts">
	import type { Evaluation } from '#lib/api';
	import { categoryLabel, severityLabel } from '#lib/labels';
	import { card, muted, small } from '#lib/ui';
	import VerdictBadge from './VerdictBadge.svelte';

	// The reference version is always a possible improved answer, never a score.
	let { evaluation }: { evaluation: Evaluation } = $props();
</script>

<section aria-label="Feedback on your answer">
	<div class={card}>
		<div class="mb-[0.6rem]">
			<VerdictBadge verdict={evaluation.verdict} />
		</div>
		<p class="mb-[0.4rem] text-[1.05rem]">{evaluation.summary}</p>
		{#if evaluation.confidence !== 'high'}
			<p class={`${small} ${muted}`}>
				The AI is {evaluation.confidence === 'low' ? 'not very' : 'only fairly'} sure about this feedback.
				Double check anything that looks odd.
			</p>
		{/if}
	</div>

	{#if evaluation.strengths.length}
		<div class={card}>
			<h3 class="mb-2 text-base font-bold">What you did well</h3>
			<ul class="m-0 pl-5">
				{#each evaluation.strengths as s, i (i)}
					<li class={i > 0 ? 'mt-[0.4rem]' : undefined}>{s}</li>
				{/each}
			</ul>
		</div>
	{/if}

	{#if evaluation.issues.length}
		<div class={card}>
			<h3 class="mb-2 text-base font-bold">What to improve</h3>
			<ol class="m-0 pl-[1.4rem]">
				{#each evaluation.issues as issue, i (i)}
					<li class="mb-4 pl-[0.2rem] last:mb-0">
						<p class={`${small} mb-1 flex flex-wrap gap-[0.6rem]`}>
							<strong>{categoryLabel(issue.category)}</strong>
							<span class={muted}>{severityLabel[issue.severity] ?? issue.severity}</span>
						</p>
						{#if issue.original || issue.correction}
							<p class="mb-1 text-[1.05rem]">
								{#if issue.original}<del class={`${muted} line-through`}>{issue.original}</del>{/if}
								{#if issue.original && issue.correction}<span aria-hidden="true"> → </span
									><span class="sr-only"> should be </span>{/if}
								{#if issue.correction}<ins
										class="font-bold underline decoration-2 underline-offset-[3px]"
										>{issue.correction}</ins
									>{/if}
							</p>
						{/if}
						<p class="m-0">{issue.explanation}</p>
					</li>
				{/each}
			</ol>
		</div>
	{:else}
		<div class={card}>
			<h3 class="mb-2 text-base font-bold">What to improve</h3>
			<p class={muted}>No mistakes were found in your answer.</p>
		</div>
	{/if}

	<div class={`${card} border-[1.5px] border-neutral-900 dark:border-neutral-100`}>
		<h3 class="mb-2 text-base font-bold">A possible improved version</h3>
		<p class="mb-[0.4rem] text-[1.1rem] font-semibold">{evaluation.improved_version}</p>
		<p class={`${small} ${muted}`}>
			Written by the AI as one example. Other wordings can be just as good.
		</p>
	</div>

	<p class={`${small} ${muted} mt-1`}>
		This feedback is written by an AI and can contain mistakes. It is practice feedback, not an
		official IELTS score.
	</p>
</section>
