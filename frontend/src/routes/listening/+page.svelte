<script lang="ts">
	import { onMount, tick } from 'svelte';
	import {
		api,
		ApiError,
		type AttemptResult,
		type Exercise,
		type ListeningMode
	} from '#lib/api';
	import { newId } from '#lib/uuid';
	import { formatDuration, userTimeZone } from '#lib/format';
	import { alert, alertTitle, btn, btnPrimary, card, muted, narrow, small } from '#lib/ui';
	import ExerciseCard from '#lib/components/ExerciseCard.svelte';
	import Feedback from '#lib/components/Feedback.svelte';
	import {
		bestVoice,
		cancelSpeech,
		defaultSpeechRate,
		describeVoice,
		loadVoices,
		rankVoices,
		speak,
		speechRates,
		speechSupported
	} from '#lib/speech';

	const MAX_PLAYS = 3;
	const MODE_KEY = 'ielts.listening.mode';
	const RATE_KEY = 'ielts.listening.rate';
	const VOICE_KEY = 'ielts.listening.voice';

	const modes: { value: ListeningMode; label: string; hint: string }[] = [
		{ value: 'text', label: 'Words and sentences', hint: 'Type the exact word, phrase or sentence.' },
		{ value: 'numeric', label: 'Numbers and details', hint: 'Type the number or detail you hear.' }
	];

	let mode = $state<ListeningMode>('text');
	let rate = $state(defaultSpeechRate);
	let exercise = $state<Exercise | null>(null);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let loadErrorCode = $state<string | null>(null);

	let voices = $state<SpeechSynthesisVoice[]>([]);
	let voiceURI = $state<string | null>(null);
	let voiceError = $state<string | null>(null);
	let speaking = $state(false);
	let plays = $state(0);
	// True once a playback was attempted, even if the device could not produce
	// sound. It lets the learner answer instead of being trapped on the page.
	let listenAttempted = $state(false);
	let startedAt = $state<number | null>(null);
	// Guards against a speech promise resolving after the exercise changed.
	let speechSeq = $state(0);
	// Auto-play is only safe after the learner has interacted with the page
	// once; browsers block speech before a click on some platforms.
	let interacted = $state(false);

	let answer = $state('');
	let answerInput = $state<HTMLInputElement | null>(null);
	let attemptId = $state(newId());
	let submitting = $state(false);
	let submitError = $state<ApiError | null>(null);
	let result = $state<AttemptResult | null>(null);
	let submittedAnswer = $state('');
	let elapsedMs = $state<number | null>(null);

	const supported = $derived(speechSupported());
	const chosenVoice = $derived(voices.find((v) => v.voiceURI === voiceURI) ?? bestVoice(voices));
	const voiceInfo = $derived(describeVoice(chosenVoice));
	const englishVoices = $derived(rankVoices(voices).slice(0, 12));
	const playsLeft = $derived(Math.max(0, MAX_PLAYS - plays));
	const canSubmit = $derived(
		!submitting && answer.trim().length > 0 && !result && (!supported || listenAttempted)
	);
	const modeHint = $derived(modes.find((m) => m.value === mode)?.hint ?? '');

	onMount(() => {
		try {
			const savedMode = localStorage.getItem(MODE_KEY);
			if (savedMode === 'text' || savedMode === 'numeric') mode = savedMode;
			const savedRate = Number(localStorage.getItem(RATE_KEY));
			if (speechRates.some((r) => r.value === savedRate)) rate = savedRate;
			voiceURI = localStorage.getItem(VOICE_KEY);
		} catch {
			/* storage can be unavailable; the defaults are fine */
		}
		loadNext();
		if (supported) {
			loadVoices().then((list) => {
				voices = list;
				// A stored voice may no longer exist on this device.
				if (voiceURI && !list.some((v) => v.voiceURI === voiceURI)) voiceURI = null;
				if (!list.length) {
					voiceError = 'This browser has no English speech voice installed.';
				}
			});
		}
		return cancelSpeech;
	});

	function reset(fresh: boolean) {
		answer = '';
		attemptId = newId();
		result = null;
		submitError = null;
		plays = 0;
		listenAttempted = false;
		startedAt = null;
		elapsedMs = null;
		speaking = false;
		speechSeq += 1;
		cancelSpeech();
		if (fresh) {
			loadError = null;
			loadErrorCode = null;
		}
	}

	async function loadNext() {
		loading = true;
		loadError = null;
		loadErrorCode = null;
		reset(true);
		try {
			exercise = await api.nextListening(mode, exercise?.id, userTimeZone());
			await tick();
			answerInput?.focus();
			// After the first manual play, later items can start automatically:
			// the Next button already counted as a user gesture.
			if (interacted && supported) await play();
		} catch (e) {
			exercise = null;
			loadError = e instanceof ApiError ? e.message : 'Could not load a recording.';
			loadErrorCode = e instanceof ApiError ? e.code : null;
		} finally {
			loading = false;
		}
	}

	async function play() {
		if (!exercise?.audio_text) return;
		if (!supported) {
			voiceError = 'This browser cannot play speech. Try Chrome, Edge or Safari.';
			return;
		}
		if (speaking) return;
		interacted = true;
		listenAttempted = true;
		voiceError = null;
		speaking = true;
		const seq = speechSeq;
		if (startedAt === null) startedAt = Date.now();
		try {
			// The first click may happen before Chrome has populated its voice
			// list. Wait for it once, so the very first recording already uses
			// the best voice instead of the default robotic one.
			if (voices.length === 0) voices = await loadVoices();
			const voice = voices.find((v) => v.voiceURI === voiceURI) ?? bestVoice(voices);
			await speak(exercise.audio_text, { voice, rate });
			if (seq === speechSeq) plays += 1;
		} catch (e) {
			if (seq === speechSeq) {
				voiceError = e instanceof Error ? e.message : 'The recording could not be played.';
			}
		} finally {
			if (seq === speechSeq) {
				speaking = false;
				await tick();
				answerInput?.focus();
			}
		}
	}

	function changeMode(next: ListeningMode) {
		if (next === mode) return;
		mode = next;
		try {
			localStorage.setItem(MODE_KEY, next);
		} catch {
			/* ignore */
		}
		loadNext();
	}

	function changeRate(next: number) {
		rate = next;
		try {
			localStorage.setItem(RATE_KEY, String(next));
		} catch {
			/* ignore */
		}
	}

	function changeVoice(uri: string) {
		voiceURI = uri || null;
		try {
			if (voiceURI) localStorage.setItem(VOICE_KEY, voiceURI);
			else localStorage.removeItem(VOICE_KEY);
		} catch {
			/* ignore */
		}
	}

	async function submit() {
		if (!exercise || !canSubmit) return;
		const text = answer.trim();
		submitting = true;
		submitError = null;
		const duration = startedAt !== null ? Date.now() - startedAt : undefined;
		elapsedMs = duration ?? null;
		try {
			result = await api.submitAttempt({
				id: attemptId,
				exercise_id: exercise.id,
				answer: text,
				duration_ms: duration,
				replays: Math.max(0, plays - 1)
			});
			submittedAnswer = text;
		} catch (e) {
			submitError =
				e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Something went wrong. Try again.', true);
		} finally {
			submitting = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			submit();
		}
	}

	async function next() {
		interacted = true;
		await loadNext();
	}
</script>

<svelte:head><title>Listening · IELTS Practice</title></svelte:head>

<div class={narrow}>
	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<h1 class="m-0 text-[1.75rem] font-bold tracking-[-0.01em]">Listening</h1>
		<div
			class="inline-flex overflow-hidden rounded-full border-[1.5px] border-neutral-900 dark:border-neutral-100"
			role="group"
			aria-label="Dictation type"
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
		<div class={card} aria-busy="true"><p class={muted}>Loading a recording…</p></div>
	{:else if loadError}
		<div class={alert} role="alert">
			<strong class={alertTitle}>
				{#if loadErrorCode === 'daily_limit_reached'}Daily listening limit reached
				{:else if loadErrorCode === 'nothing_due'}Nothing due right now
				{:else}Could not load a recording{/if}
			</strong>
			<span>{loadError}</span>
		</div>
		<div class="flex flex-wrap gap-[0.6rem]">
			<button class={btn} type="button" onclick={loadNext}>Try again</button>
			<a class={btn} href="/">Back to practice</a>
		</div>
	{:else if exercise}
		<ExerciseCard {exercise} />

		<section class={card} aria-label="Recording">
			<div class="flex flex-wrap items-center gap-3">
				<button
					class={`${btnPrimary} inline-flex items-center gap-2 text-[1.05rem]`}
					type="button"
					onclick={play}
					disabled={speaking || playsLeft === 0}
				>
					<svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true" fill="currentColor">
						<path d="M8 5v14l11-7z" />
					</svg>
					{#if speaking}
						Playing…
					{:else if plays === 0}
						Play the recording
					{:else if playsLeft === 0}
						No replays left
					{:else}
						Play again ({playsLeft} left)
					{/if}
				</button>
				<span class={`${small} ${muted}`}>
					{#if plays === 0}
						Listen carefully, then type exactly what you hear.
					{:else}
						Listened {plays} of {MAX_PLAYS} times.
					{/if}
				</span>
			</div>

			<div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2">
				<div class="flex items-center gap-2">
					<span class={`${small} font-semibold`}>Speed</span>
					<div class="inline-flex overflow-hidden rounded-full border-[1.5px] border-neutral-900 dark:border-neutral-100">
						{#each speechRates as r (r.value)}
							<button
								type="button"
								class="cursor-pointer border-0 bg-transparent px-3 py-[0.25rem] text-[0.85rem] font-semibold text-neutral-900 aria-pressed:bg-neutral-900 aria-pressed:text-white dark:text-neutral-100 dark:aria-pressed:bg-neutral-100 dark:aria-pressed:text-neutral-900"
								aria-pressed={rate === r.value}
								onclick={() => changeRate(r.value)}>{r.label}</button
							>
						{/each}
					</div>
				</div>

				{#if englishVoices.length > 1}
					<label class={`${small} flex items-center gap-2`}>
						<span class="font-semibold">Voice</span>
						<select
							class="max-w-[16rem] rounded-[8px] border-[1.5px] border-neutral-300 bg-white px-2 py-[0.3rem] text-[0.9rem] text-neutral-900 dark:border-neutral-700 dark:bg-neutral-950 dark:text-neutral-100"
							value={voiceURI ?? ''}
							onchange={(e) => changeVoice((e.currentTarget as HTMLSelectElement).value)}
						>
							<option value="">Automatic (best available)</option>
							{#each englishVoices as v (v.voiceURI)}
								<option value={v.voiceURI}>{v.name} ({v.lang})</option>
							{/each}
						</select>
					</label>
				{/if}
			</div>

			{#if voiceInfo}
				<p class={`${small} ${muted} mt-3`}>
					Using <strong>{voiceInfo.name}</strong> ({voiceInfo.lang}{voiceInfo.natural
						? ', natural voice'
						: ', device voice'}).
					{#if !voiceInfo.natural}
						For the most natural pronunciation, install an enhanced or natural English voice in your
						operating system's speech settings, then reload this page.
					{/if}
				</p>
			{/if}
			{#if voiceError}
				<div class={alert} role="alert">
					<strong class={alertTitle}>Audio problem</strong><span>{voiceError}</span>
				</div>
			{/if}
		</section>

		{#if !result}
			<form
				onsubmit={(e) => {
					e.preventDefault();
					submit();
				}}
			>
				<label for="dictation" class="mb-[0.4rem] block font-semibold">
					{mode === 'numeric' ? 'The detail you heard' : 'What you heard'}
				</label>
				<input
					id="dictation"
					class="w-full rounded-[10px] border-[1.5px] border-neutral-300 bg-white px-[0.9rem] py-[0.8rem] font-sans text-[1.15rem] leading-[1.4] text-neutral-900 focus-visible:border-neutral-900 disabled:opacity-60 dark:border-neutral-700 dark:bg-neutral-950 dark:text-neutral-100 dark:focus-visible:border-neutral-100"
					bind:this={answerInput}
					bind:value={answer}
					onkeydown={onKeydown}
					disabled={submitting}
					autocomplete="off"
					autocapitalize="off"
					spellcheck="false"
					placeholder={mode === 'numeric' ? 'Type the number or detail…' : 'Type the words you heard…'}
					aria-describedby="dictation-help"
				/>
				<p id="dictation-help" class={`${small} ${muted} mx-0 mb-4 mt-[0.35rem]`}>
					{modeHint}
					{#if supported && !listenAttempted}
						Play the recording first.
					{:else}
						Press Enter to check.
					{/if}
				</p>

				{#if submitError}
					<div class={alert} role="alert">
						<strong class={alertTitle}
							>{submitError.retryable ? 'Not evaluated yet' : 'Could not submit'}</strong
						>
						<span>{submitError.message}</span>
					</div>
				{/if}

				<div class="flex flex-wrap gap-[0.6rem]">
					<button class={btnPrimary} type="submit" disabled={!canSubmit}>
						{submitError?.retryable ? 'Try again' : 'Check my answer'}
					</button>
					<button class={btn} type="button" onclick={next} disabled={submitting || speaking}>
						Skip to new recording
					</button>
				</div>
				<p class={`${small} ${muted} mt-[0.6rem] min-h-6`} role="status" aria-live="polite">
					{submitting ? 'Checking your transcription. This usually takes a few seconds.' : ''}
				</p>
			</form>
		{:else}
			<section class={card} aria-label="Your answer">
				<h3 class="mb-2 text-base font-bold">Your transcription</h3>
				<p class="whitespace-pre-wrap break-words text-[1.1rem]">{submittedAnswer}</p>
				{#if elapsedMs !== null}
					<p class={`${small} ${muted} mt-2`}>
						Took {formatDuration(elapsedMs)} · {plays} listen{plays === 1 ? '' : 's'}
					</p>
				{/if}
			</section>

			<section class={card} aria-label="Reference transcript">
				<h3 class="mb-2 text-base font-bold">What was said</h3>
				<p class="mb-[0.4rem] text-[1.1rem] font-semibold">{result.evaluation.improved_version}</p>
				<div class="flex flex-wrap items-center gap-[0.6rem]">
					<button class={btn} type="button" onclick={play} disabled={speaking}>
						{speaking ? 'Playing…' : 'Listen again'}
					</button>
					<span class={`${small} ${muted}`}>Listen and compare with what you typed.</span>
				</div>
			</section>

			<Feedback evaluation={result.evaluation} showReference={false} />

			<div class="mb-3 mt-4 flex flex-wrap gap-[0.6rem]">
				<button class={btnPrimary} type="button" onclick={next}>Next recording</button>
			</div>
		{/if}
	{/if}
</div>
