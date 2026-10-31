<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api, ApiError, type AttemptDetail } from '#lib/api';
	import { formatDateTime } from '#lib/format';
	import { alert, alertTitle, answerText, btnPrimary, card, muted, narrow, small } from '#lib/ui';
	import ExerciseCard from '#lib/components/ExerciseCard.svelte';
	import Feedback from '#lib/components/Feedback.svelte';

	let detail = $state<AttemptDetail | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let retrying = $state(false);
	let retryError = $state<string | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			detail = await api.getAttempt(page.params.id ?? '');
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not load this attempt.';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	// A saved answer whose evaluation failed can be evaluated again. The same
	// attempt id is reused, so it never creates a duplicate.
	async function retryEvaluation() {
		if (!detail) return;
		retrying = true;
		retryError = null;
		try {
			await api.submitAttempt({
				id: detail.id,
				exercise_id: detail.exercise.id,
				answer: detail.answer,
				parent_attempt_id: detail.parent_attempt_id
			});
			await load();
		} catch (e) {
			retryError = e instanceof ApiError ? e.message : 'Could not evaluate this answer.';
		} finally {
			retrying = false;
		}
	}
</script>

<svelte:head><title>Attempt · IELTS Practice</title></svelte:head>

<div class={narrow}>
	<p><a href="/history">← Back to history</a></p>

	{#if loading}
		<div class={card} aria-busy="true"><p class={muted}>Loading…</p></div>
	{:else if error}
		<div class={alert} role="alert">
			<strong class={alertTitle}>Problem</strong><span>{error}</span>
		</div>
	{:else if detail}
		<h1 class="mb-2 text-[1.75rem] font-bold tracking-[-0.01em]">Attempt</h1>
		<p class={`${small} ${muted}`}>
			{formatDateTime(detail.created_at)}
			{#if detail.parent_attempt_id}
				· Rewrite of <a href="/history/{detail.parent_attempt_id}">an earlier answer</a>
			{/if}
		</p>

		<ExerciseCard exercise={detail.exercise} />

		<section class={card} aria-label="Your answer">
			<h3 class="mb-2 text-base font-bold">Your answer</h3>
			<p class={`${answerText} m-0`}>{detail.answer}</p>
		</section>

		{#if detail.status === 'evaluated' && detail.evaluation}
			<Feedback evaluation={detail.evaluation} />
			{#if detail.reference_meaning}
				<section class={card}>
					<h3 class="mb-2 text-base font-bold">Reference meaning</h3>
					<p>{detail.reference_meaning}</p>
					<p class={`${small} ${muted}`}>
						Used by the evaluator as a guide. Other correct explanations are accepted.
					</p>
				</section>
			{/if}
		{:else}
			<div class={alert}>
				<strong class={alertTitle}>Saved, but not evaluated</strong>
				<span>
					Your answer is stored safely. The AI could not evaluate it at the time. You can ask for the
					evaluation again.
				</span>
			</div>
			{#if retryError}
				<div class={alert} role="alert">
					<strong class={alertTitle}>Still not evaluated</strong><span>{retryError}</span>
				</div>
			{/if}
			<button class={btnPrimary} type="button" disabled={retrying} onclick={retryEvaluation}>
				{retrying ? 'Evaluating…' : 'Evaluate this answer now'}
			</button>
		{/if}
	{/if}
</div>
