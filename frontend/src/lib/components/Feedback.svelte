<script lang="ts">
	import type { Evaluation } from '#lib/api';
	import { categoryLabel, severityLabel } from '#lib/labels';
	import VerdictBadge from './VerdictBadge.svelte';

	let { evaluation }: { evaluation: Evaluation } = $props();
</script>

<section class="feedback" aria-label="Feedback on your answer">
	<div class="card">
		<div class="head">
			<VerdictBadge verdict={evaluation.verdict} />
		</div>
		<p class="summary">{evaluation.summary}</p>
		{#if evaluation.confidence !== 'high'}
			<p class="small muted">
				The AI is {evaluation.confidence === 'low' ? 'not very' : 'only fairly'} sure about this feedback.
				Double check anything that looks odd.
			</p>
		{/if}
	</div>

	{#if evaluation.strengths.length}
		<div class="card">
			<h3>What you did well</h3>
			<ul>
				{#each evaluation.strengths as s, i (i)}<li>{s}</li>{/each}
			</ul>
		</div>
	{/if}

	{#if evaluation.issues.length}
		<div class="card">
			<h3>What to improve</h3>
			<ol class="issues">
				{#each evaluation.issues as issue, i (i)}
					<li>
						<p class="tags small">
							<strong>{categoryLabel(issue.category)}</strong>
							<span class="muted">{severityLabel[issue.severity] ?? issue.severity}</span>
						</p>
						{#if issue.original || issue.correction}
							<p class="change">
								{#if issue.original}<del>{issue.original}</del>{/if}
								{#if issue.original && issue.correction}<span aria-hidden="true"> → </span
									><span class="visually-hidden"> should be </span>{/if}
								{#if issue.correction}<ins>{issue.correction}</ins>{/if}
							</p>
						{/if}
						<p class="explanation">{issue.explanation}</p>
					</li>
				{/each}
			</ol>
		</div>
	{:else}
		<div class="card">
			<h3>What to improve</h3>
			<p class="muted">No mistakes were found in your answer.</p>
		</div>
	{/if}

	<div class="card improved">
		<h3>A possible improved version</h3>
		<p class="improved-text">{evaluation.improved_version}</p>
		<p class="small muted">Written by the AI as one example. Other wordings can be just as good.</p>
	</div>

	<p class="small muted disclaimer">
		This feedback is written by an AI and can contain mistakes. It is practice feedback, not an
		official IELTS score.
	</p>
</section>

<style>
	.head {
		margin-bottom: 0.6rem;
	}
	.summary {
		font-size: 1.05rem;
		margin-bottom: 0.4rem;
	}
	ul,
	ol {
		margin: 0;
		padding-left: 1.2rem;
	}
	li + li {
		margin-top: 0.4rem;
	}
	.issues {
		padding-left: 1.4rem;
	}
	.issues li {
		margin-bottom: 1rem;
		padding-left: 0.2rem;
	}
	.issues li:last-child {
		margin-bottom: 0;
	}
	.tags {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
		margin-bottom: 0.25rem;
	}
	.change {
		font-size: 1.05rem;
		margin-bottom: 0.25rem;
	}
	del {
		text-decoration: line-through;
		color: var(--muted);
	}
	ins {
		text-decoration: underline;
		text-decoration-thickness: 2px;
		text-underline-offset: 3px;
		font-weight: 700;
	}
	.explanation {
		margin: 0;
	}
	.improved {
		border-color: var(--strong);
		border-width: 1.5px;
	}
	.improved-text {
		font-size: 1.1rem;
		font-weight: 600;
		margin-bottom: 0.4rem;
	}
	.disclaimer {
		margin-top: 0.25rem;
	}
</style>
