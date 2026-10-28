<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		api,
		ApiError,
		type AttemptResult,
		type Evaluation,
		type Exercise,
		type ExerciseType
	} from '#lib/api';
	import { newId } from '#lib/uuid';
	import { userTimeZone } from '#lib/format';
	import ExerciseCard from '#lib/components/ExerciseCard.svelte';
	import Feedback from '#lib/components/Feedback.svelte';

	type Mode = 'any' | ExerciseType;
	const MAX_CHARS = 1000;
	const MODE_KEY = 'ielts.mode';

	const modes: { value: Mode; label: string }[] = [
		{ value: 'any', label: 'Mixed' },
		{ value: 'vocab', label: 'Vocabulary' },
		{ value: 'writing', label: 'Writing' }
	];

	let mode = $state<Mode>('any');
	let exercise = $state<Exercise | null>(null);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let loadErrorCode = $state<string | null>(null);

	let answer = $state('');
	let attemptId = $state(newId());
	let parentId = $state<string | null>(null);
	let submitting = $state(false);
	let submitError = $state<ApiError | null>(null);
	// The text of the last submission that failed. If the learner edits the text
	// afterwards, a new attempt id is needed, because an id is tied to one answer.
	let failedText = $state<string | null>(null);

	let result = $state<AttemptResult | null>(null);
	let submittedAnswer = $state('');
	let previous = $state<{ answer: string; evaluation: Evaluation } | null>(null);

	let textarea = $state<HTMLTextAreaElement | null>(null);

	// Count characters the way the server does (code points, not UTF-16 units).
	const chars = $derived([...answer].length);
	const canSubmit = $derived(!submitting && answer.trim().length > 0 && chars <= MAX_CHARS);

	onMount(() => {
		try {
			const saved = localStorage.getItem(MODE_KEY);
			if (saved === 'any' || saved === 'vocab' || saved === 'writing') mode = saved;
		} catch {
			/* storage can be unavailable; the default is fine */
		}
		loadNext();
	});

	function reset() {
		answer = '';
		attemptId = newId();
		parentId = null;
		result = null;
		previous = null;
		submitError = null;
		failedText = null;
	}

	async function loadNext() {
		loading = true;
		loadError = null;
		loadErrorCode = null;
		reset();
		try {
			exercise = await api.nextExercise(mode, exercise?.id, userTimeZone());
			await tick();
			textarea?.focus();
		} catch (e) {
			exercise = null;
			loadError = e instanceof ApiError ? e.message : 'Could not load an exercise.';
			loadErrorCode = e instanceof ApiError ? e.code : null;
		} finally {
			loading = false;
		}
	}

	function changeMode(next: Mode) {
		if (next === mode) return;
		mode = next;
		try {
			localStorage.setItem(MODE_KEY, next);
		} catch {
			/* ignore */
		}
		loadNext();
	}

	async function submit() {
		if (!exercise || !canSubmit) return;
		const text = answer.trim();
		if (failedText !== null && failedText !== text) attemptId = newId();
		submitting = true;
		submitError = null;
		try {
			result = await api.submitAttempt({
				id: attemptId,
				exercise_id: exercise.id,
				answer: text,
				parent_attempt_id: parentId
			});
			submittedAnswer = text;
			failedText = null;
		} catch (e) {
			failedText = text;
			submitError =
				e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Something went wrong. Try again.', true);
		} finally {
			submitting = false;
		}
	}

	async function startRewrite() {
		if (!result) return;
		previous = { answer: submittedAnswer, evaluation: result.evaluation };
		parentId = result.attempt_id;
		attemptId = newId();
		answer = '';
		result = null;
		submitError = null;
		failedText = null;
		await tick();
		textarea?.focus();
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
			e.preventDefault();
			submit();
		}
	}
</script>

<svelte:head><title>Practice · IELTS Practice</title></svelte:head>

<div class="narrow">
	<div class="top">
		<h1>Practice</h1>
		<div class="modes" role="group" aria-label="Exercise type">
			{#each modes as m (m.value)}
				<button
					type="button"
					class="mode"
					aria-pressed={mode === m.value}
					onclick={() => changeMode(m.value)}>{m.label}</button
				>
			{/each}
		</div>
	</div>

	{#if loading}
		<div class="card" aria-busy="true"><p class="muted">Loading an exercise…</p></div>
	{:else if loadError}
		<div class="alert" role="alert">
			<strong>
				{#if loadErrorCode === 'daily_limit_reached'}New-word limit reached
				{:else if loadErrorCode === 'nothing_due'}Nothing due right now
				{:else}Could not load an exercise{/if}
			</strong>
			<span>{loadError}</span>
		</div>
		<div class="actions">
			{#if loadErrorCode === 'daily_limit_reached' || loadErrorCode === 'nothing_due'}
				<button class="btn primary" type="button" onclick={() => changeMode('writing')}>
					Practice writing instead
				</button>
			{/if}
			<button class="btn" type="button" onclick={loadNext}>Try again</button>
		</div>
	{:else if exercise}
		<ExerciseCard {exercise} />

		{#if previous}
			<details class="card previous">
				<summary>Your first answer and its feedback</summary>
				<p class="answer-text">{previous.answer}</p>
				<Feedback evaluation={previous.evaluation} />
			</details>
		{/if}

		{#if !result}
			<form
				onsubmit={(e) => {
					e.preventDefault();
					submit();
				}}
			>
				<label for="answer" class="label">
					{#if previous}Now write it again in your own words{:else}Your answer{/if}
				</label>
				<textarea
					id="answer"
					bind:this={textarea}
					bind:value={answer}
					onkeydown={onKeydown}
					disabled={submitting}
					rows="5"
					placeholder={exercise.type === 'vocab'
						? 'Explain the word in English, in your own words…'
						: 'Write your answer in English…'}
					aria-describedby="counter"
				></textarea>
				<div class="row">
					<span id="counter" class="small muted" class:over={chars > MAX_CHARS}>
						{chars} / {MAX_CHARS}
					</span>
					<span class="small muted hint">Ctrl or Cmd + Enter to submit</span>
				</div>

				{#if submitError}
					<div class="alert" role="alert">
						<strong>{submitError.retryable ? 'Not evaluated yet' : 'Could not submit'}</strong>
						<span>{submitError.message}</span>
					</div>
				{/if}

				<div class="actions">
					<button class="btn primary" type="submit" disabled={!canSubmit}>
						{submitError?.retryable ? 'Try again' : 'Get feedback'}
					</button>
					{#if !previous}
						<button class="btn" type="button" onclick={loadNext} disabled={submitting}>
							Skip this exercise
						</button>
					{/if}
				</div>
				<p class="small muted status" role="status" aria-live="polite">
					{#if submitting}Evaluating your answer. This usually takes a few seconds.{/if}
				</p>
			</form>
		{:else}
			<section class="card" aria-label="Your answer">
				<h3>Your answer</h3>
				<p class="answer-text">{submittedAnswer}</p>
			</section>

			<Feedback evaluation={result.evaluation} />

			<div class="actions after">
				<button class="btn primary" type="button" onclick={startRewrite}>
					Rewrite it in my own words
				</button>
				<button class="btn" type="button" onclick={loadNext}>Next exercise</button>
			</div>
			<p class="small muted">
				Rewriting is optional but useful: producing the corrected form yourself is what builds
				active English.
			</p>
		{/if}
	{/if}
</div>

<style>
	.narrow {
		max-width: var(--content);
	}
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}
	.top h1 {
		margin: 0;
	}
	.modes {
		display: inline-flex;
		border: 1.5px solid var(--strong);
		border-radius: 999px;
		overflow: hidden;
	}
	.mode {
		font: inherit;
		font-weight: 650;
		font-size: 0.9rem;
		background: transparent;
		color: var(--text);
		border: 0;
		padding: 0.4rem 0.95rem;
		cursor: pointer;
	}
	.mode[aria-pressed='true'] {
		background: var(--strong);
		color: var(--on-strong);
	}
	.label {
		display: block;
		font-weight: 650;
		margin-bottom: 0.4rem;
	}
	.row {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		margin: 0.35rem 0 1rem;
	}
	.over {
		font-weight: 700;
		color: var(--text);
		text-decoration: underline;
	}
	.hint {
		text-align: right;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
	}
	.after {
		margin: 1rem 0 0.75rem;
	}
	.status {
		min-height: 1.5rem;
		margin-top: 0.6rem;
	}
	.answer-text {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-size: 1.05rem;
	}
	.previous summary {
		cursor: pointer;
		font-weight: 650;
	}
	.previous[open] summary {
		margin-bottom: 0.75rem;
	}
</style>
