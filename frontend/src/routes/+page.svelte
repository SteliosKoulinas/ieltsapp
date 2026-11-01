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
	import {
		alert,
		alertTitle,
		answerText,
		btn,
		btnPrimary,
		card,
		muted,
		narrow,
		small,
		textarea as textareaClass
	} from '#lib/ui';
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

	// Skipping asks the server for unseen material only. When none is left, the
	// current exercise stays on screen and this message explains why.
	let skipping = $state(false);
	let skipError = $state<string | null>(null);

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
		skipError = null;
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

	// Skip must show material the learner has never seen. It never falls back to
	// a word from the review schedule, because that is what made Skip feel
	// broken: it kept returning words that had already been practised.
	async function skip() {
		if (!exercise || skipping) return;
		skipping = true;
		skipError = null;
		try {
			exercise = await api.nextExercise(mode, exercise.id, userTimeZone(), true);
			reset();
			await tick();
			textarea?.focus();
		} catch (e) {
			skipError = e instanceof ApiError ? e.message : 'Could not load new material.';
		} finally {
			skipping = false;
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

<div class={narrow}>
	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<h1 class="m-0 text-[1.75rem] font-bold tracking-[-0.01em]">Practice</h1>
		<div
			class="inline-flex overflow-hidden rounded-full border-[1.5px] border-neutral-900 dark:border-neutral-100"
			role="group"
			aria-label="Exercise type"
		>
			{#each modes as m (m.value)}
				<button
					type="button"
					class="cursor-pointer border-0 bg-transparent px-[0.95rem] py-[0.4rem] text-[0.9rem] font-semibold text-neutral-900 aria-pressed:bg-neutral-900 aria-pressed:text-white dark:text-neutral-100 dark:aria-pressed:bg-neutral-100 dark:aria-pressed:text-neutral-900"
					aria-pressed={mode === m.value}
					onclick={() => changeMode(m.value)}>{m.label}</button
				>
			{/each}
		</div>
	</div>

	{#if loading}
		<div class={card} aria-busy="true"><p class={muted}>Loading an exercise…</p></div>
	{:else if loadError}
		<div class={alert} role="alert">
			<strong class={alertTitle}>
				{#if loadErrorCode === 'daily_limit_reached'}New-word limit reached
				{:else if loadErrorCode === 'nothing_due'}Nothing due right now
				{:else}Could not load an exercise{/if}
			</strong>
			<span>{loadError}</span>
		</div>
		<div class="flex flex-wrap gap-[0.6rem]">
			{#if loadErrorCode === 'daily_limit_reached' || loadErrorCode === 'nothing_due'}
				<button class={btnPrimary} type="button" onclick={() => changeMode('writing')}>
					Practice writing instead
				</button>
			{/if}
			<button class={btn} type="button" onclick={loadNext}>Try again</button>
		</div>
	{:else if exercise}
		<ExerciseCard {exercise} />

		{#if previous}
			<details class={`${card} [&[open]_summary]:mb-3`}>
				<summary class="cursor-pointer font-semibold">Your first answer and its feedback</summary>
				<p class={answerText}>{previous.answer}</p>
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
				<label for="answer" class="mb-[0.4rem] block font-semibold">
					{#if previous}Now write it again in your own words{:else}Your answer{/if}
				</label>
				<textarea
					id="answer"
					class={textareaClass}
					bind:this={textarea}
					bind:value={answer}
					onkeydown={onKeydown}
					disabled={submitting}
					rows="5"
					placeholder="Write your answer in English…"
					aria-describedby="counter"
				></textarea>
				<div class="mx-0 mb-4 mt-[0.35rem] flex justify-between gap-4">
					<span
						id="counter"
						class={`${small} ${muted} ${chars > MAX_CHARS ? 'font-bold text-neutral-900 underline dark:text-neutral-100' : ''}`}
					>
						{chars} / {MAX_CHARS}
					</span>
					<span class={`${small} ${muted} text-right`}>Ctrl or Cmd + Enter to submit</span>
				</div>

				{#if submitError}
					<div class={alert} role="alert">
						<strong class={alertTitle}
							>{submitError.retryable ? 'Not evaluated yet' : 'Could not submit'}</strong
						>
						<span>{submitError.message}</span>
					</div>
				{/if}

				{#if skipError}
					<div class={alert} role="alert">
						<strong class={alertTitle}>No new material left</strong>
						<span>{skipError}</span>
					</div>
				{/if}

				<div class="flex flex-wrap gap-[0.6rem]">
					<button class={btnPrimary} type="submit" disabled={!canSubmit}>
						{submitError?.retryable ? 'Try again' : 'Get feedback'}
					</button>
					{#if !previous}
						<button class={btn} type="button" onclick={skip} disabled={submitting || skipping}>
							{skipping ? 'Finding new material…' : 'Skip to new material'}
						</button>
					{/if}
				</div>
				<p class={`${small} ${muted} mt-[0.6rem] min-h-6`} role="status" aria-live="polite">
					{#if submitting}Evaluating your answer. This usually takes a few seconds.{/if}
				</p>
			</form>
		{:else}
			<section class={card} aria-label="Your answer">
				<h3 class="mb-2 text-base font-bold">Your answer</h3>
				<p class={answerText}>{submittedAnswer}</p>
			</section>

			<Feedback evaluation={result.evaluation} />

			<div class="mb-3 mt-4 flex flex-wrap gap-[0.6rem]">
				<button class={btnPrimary} type="button" onclick={startRewrite}>
					Rewrite it in my own words
				</button>
				<button class={btn} type="button" onclick={loadNext}>Next exercise</button>
			</div>
			<p class={`${small} ${muted}`}>
				Rewriting is optional but useful: producing the corrected form yourself is what builds
				active English.
			</p>
		{/if}
	{/if}
</div>

