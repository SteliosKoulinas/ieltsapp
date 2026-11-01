<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type Stats, type Verdict } from '#lib/api';
	import { categoryLabel, typeLabel, verdictLabel, verdictOrder } from '#lib/labels';
	import { dayMonth, formatDateTime, percent, userTimeZone, weekday } from '#lib/format';
	import {
		alert,
		alertTitle,
		btnPrimary,
		card,
		muted,
		num,
		panel,
		small,
		table,
		td,
		th,
		verdictPattern
	} from '#lib/ui';

	let stats = $state<Stats | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			stats = await api.stats(userTimeZone());
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not load statistics.';
		} finally {
			loading = false;
		}
	});

	const maxDaily = $derived(Math.max(1, ...(stats?.daily.map((d) => d.count) ?? [1])));
	const activeDays = $derived(stats?.daily.filter((d) => d.count > 0).length ?? 0);
	const answersLast14 = $derived(stats?.daily.reduce((n, d) => n + d.count, 0) ?? 0);

	// Overall verdict counts across both exercise types (first attempts only).
	const overall = $derived.by(() => {
		const out: Record<Verdict, number> = { correct: 0, mostly_correct: 0, partially_correct: 0, incorrect: 0 };
		for (const t of stats?.by_type ?? []) {
			for (const v of verdictOrder) out[v] += t.verdicts[v] ?? 0;
		}
		return out;
	});

	function trendWord(total: number, recent: number): string {
		if (total === 0) return '';
		const diff = Math.round((recent - total) * 10) / 10;
		if (Math.abs(diff) < 1) return 'about the same';
		return diff < 0 ? 'lower recently' : 'higher recently';
	}
</script>

<svelte:head><title>Statistics · IELTS Practice</title></svelte:head>

<h1 class="mb-2 text-[1.75rem] font-bold tracking-[-0.01em]">Statistics</h1>

{#if loading}
	<div class={card} aria-busy="true"><p class={muted}>Loading…</p></div>
{:else if error}
	<div class={alert} role="alert">
		<strong class={alertTitle}>Problem</strong><span>{error}</span>
	</div>
{:else if stats}
	{#if stats.totals.first_attempts === 0}
		<div class={card}>
			<p>
				There is nothing to show yet. Answer a few exercises and your statistics will appear here.
			</p>
			<a class={btnPrimary} href="/">Start practising</a>
		</div>
	{:else}
		<div class="mb-3 grid grid-cols-[repeat(auto-fit,minmax(9rem,1fr))] gap-3">
			<div class={`${panel} flex flex-col`}>
				<span class="text-[2rem] font-extrabold leading-[1.1]">{stats.totals.evaluated}</span>
				<span class={`${small} ${muted}`}>answers evaluated</span>
			</div>
			<div class={`${panel} flex flex-col`}>
				<span class="text-[2rem] font-extrabold leading-[1.1]">{stats.totals.first_attempts}</span>
				<span class={`${small} ${muted}`}>exercises attempted</span>
			</div>
			<div class={`${panel} flex flex-col`}>
				<span class="text-[2rem] font-extrabold leading-[1.1]">{stats.totals.rewrites}</span>
				<span class={`${small} ${muted}`}>rewrites</span>
			</div>
			<div class={`${panel} flex flex-col`}>
				<span class="text-[2rem] font-extrabold leading-[1.1]">{stats.streak_days}</span>
				<span class={`${small} ${muted}`}
					>day{stats.streak_days === 1 ? '' : 's'} in a row</span
				>
			</div>
		</div>
		{#if stats.totals.failed > 0}
			<p class={`${small} ${muted}`}>
				{stats.totals.failed} answer{stats.totals.failed === 1 ? ' was' : 's were'} saved but not evaluated. Open
				them from <a href="/history">History</a> to evaluate again. They are not counted below.
			</p>
		{/if}

		{#if stats.first_attempts_evaluated > 0 && stats.first_attempts_evaluated < 10}
			<div class={alert}>
				<strong class={alertTitle}
					>Only {stats.first_attempts_evaluated} evaluated answer{stats.first_attempts_evaluated === 1
						? ''
						: 's'} so far</strong
				>
				<span
					>Treat the numbers below as a first impression, not a conclusion. They become more meaningful
					after about 10 to 20 answers.</span
				>
			</div>
		{/if}

		<section class={card}>
			<h2 class="mb-2 text-xl font-bold">Vocabulary schedule</h2>
			<p class={`${small} ${muted}`}>
				Words return after a waiting time that grows each time you answer them correctly. A wrong
				answer brings the word back after a few hours. Due reviews always come first and are never
				limited. New words are unlimited by default; you can set a daily cap if you want one.
			</p>
			{#if stats.srs.tracked === 0}
				<p class={muted}>No words are in the schedule yet. Answer a vocabulary exercise to start.</p>
			{:else}
				<div class="mb-3 grid grid-cols-[repeat(auto-fit,minmax(9rem,1fr))] gap-3">
					<div class={`${panel} flex flex-col`}>
						<span class="text-[2rem] font-extrabold leading-[1.1]">{stats.srs.due_now}</span>
						<span class={`${small} ${muted}`}>due now</span>
					</div>
					<div class={`${panel} flex flex-col`}>
						<span class="text-[2rem] font-extrabold leading-[1.1]"
							>{stats.srs.new_today}{stats.srs.new_per_day > 0
								? ` / ${stats.srs.new_per_day}`
								: ''}</span
						>
						<span class={`${small} ${muted}`}
							>new today{stats.srs.new_per_day > 0 ? '' : ' (no limit)'}</span
						>
					</div>
					<div class={`${panel} flex flex-col`}>
						<span class="text-[2rem] font-extrabold leading-[1.1]">{stats.srs.tracked}</span>
						<span class={`${small} ${muted}`}>words tracked</span>
					</div>
					<div class={`${panel} flex flex-col`}>
						<span class="text-[2rem] font-extrabold leading-[1.1]">{stats.srs.mastered}</span>
						<span class={`${small} ${muted}`}>in the last box</span>
					</div>
				</div>
				<div class="overflow-x-auto">
					<table class={table}>
						<thead>
							<tr>
								<th class={th}>Box</th>
								<th class={`${th} ${num}`}>Words</th>
								<th class={th}>Next review</th>
							</tr>
						</thead>
						<tbody>
							{#each stats.srs.boxes as b (b.box)}
								<tr>
									<td class={td}>{b.box}</td>
									<td class={`${td} ${num}`}>{b.count}</td>
									<td class={`${td} ${small} ${muted}`}>
										{#if b.box === 1}4 hours
										{:else if b.box === 2}1 day
										{:else if b.box === 3}3 days
										{:else if b.box === 4}8 days
										{:else}21 days{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
				{#if stats.srs.next_due_at}
					<p class={`${small} ${muted}`}>Next review: {formatDateTime(stats.srs.next_due_at)}.</p>
				{/if}
				<p class={`${small} ${muted}`}>
					A correct answer moves the word up two boxes and at least 1 day away. A mostly correct
					answer moves it up one box and at least 1 day away. A partly correct answer moves it down
					one box but returns within 1 day. A wrong answer restarts it at box 1 and returns after 4
					hours.
				</p>
			{/if}
		</section>

		<section class={card}>
			<h2 class="mb-2 text-xl font-bold">How your first attempts went</h2>
			<p class={`${small} ${muted}`}>
				Only the first answer to each exercise counts here, so rewrites made after feedback do not
				inflate the picture.
			</p>
			{#if stats.first_attempts_evaluated === 0}
				<p class={muted}>No evaluated first attempts yet.</p>
			{:else}
				<div
					class="mb-3 mt-2 flex h-[1.6rem] overflow-hidden rounded-full border-[1.5px] border-neutral-900 dark:border-neutral-100"
					role="img"
					aria-label="Share of results by verdict"
				>
					{#each verdictOrder as v (v)}
						{#if overall[v] > 0}
							<div
								class={`h-full ${verdictPattern[v]}`}
								style="flex: {overall[v]}"
								title="{verdictLabel[v]}: {overall[v]}"
							></div>
						{/if}
					{/each}
				</div>
				<ul class="mb-4 flex list-none flex-wrap gap-x-5 gap-y-[0.4rem] p-0">
					{#each verdictOrder as v (v)}
						<li>
							<span
								class={`mr-1 inline-block h-[0.9rem] w-[0.9rem] rounded-[3px] border-[1.5px] border-neutral-900 align-[-0.1em] text-neutral-900 dark:border-neutral-100 dark:text-neutral-100 ${verdictPattern[v]}`}
								aria-hidden="true"
							></span>
							{verdictLabel[v]}: <strong>{overall[v]}</strong>
							<span class={muted}>({percent(overall[v], stats.first_attempts_evaluated)}%)</span>
						</li>
					{/each}
				</ul>

				{#if stats.by_type.length > 0}
					<div class="overflow-x-auto">
						<table class={table}>
							<thead>
								<tr>
									<th class={th}>Exercise type</th>
									<th class={`${th} ${num}`}>Answers</th>
									{#each verdictOrder as v (v)}<th class={`${th} ${num}`}>{verdictLabel[v]}</th>{/each}
								</tr>
							</thead>
							<tbody>
								{#each stats.by_type as t (t.type)}
									<tr>
										<td class={td}>{typeLabel[t.type]}</td>
										<td class={`${td} ${num}`}>{t.total}</td>
										{#each verdictOrder as v (v)}<td class={`${td} ${num}`}>{t.verdicts[v] ?? 0}</td>{/each}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			{/if}
		</section>

		<section class={card}>
			<h2 class="mb-2 text-xl font-bold">Practice in the last 14 days</h2>
			<p class={`${small} ${muted}`}>
				{answersLast14} answer{answersLast14 === 1 ? '' : 's'} on {activeDays} of 14 days.
			</p>
			<div
				class="mt-3 flex h-36 items-end gap-[0.35rem]"
				role="img"
				aria-label="Answers evaluated per day, last 14 days"
			>
				{#each stats.daily as d (d.date)}
					<div
						class="flex h-full min-w-0 flex-1 flex-col items-stretch justify-end text-center"
						title="{dayMonth(d.date)}: {d.count}"
					>
						<span class={`${small} min-h-[1.3em]`}>{d.count || ''}</span>
						<div
							class="min-h-[2px] rounded-t-[4px] bg-neutral-900 dark:bg-neutral-100"
							style="height: {(d.count / maxDaily) * 100}%"
						></div>
						<span class={`${small} ${muted} mt-[0.2rem]`}>{weekday(d.date)}</span>
					</div>
				{/each}
			</div>
		</section>

		<section class={card}>
			<h2 class="mb-2 text-xl font-bold">Your most frequent mistakes</h2>
			<p class={`${small} ${muted}`}>
				Only items marked as real mistakes in first attempts are counted, as a rate per 10 answers so
				the numbers do not depend on how much you wrote. "Recently" means your last
				{stats.category_window} first attempts.
			</p>
			{#if stats.top_categories.length === 0}
				<p class={muted}>No mistakes recorded yet.</p>
			{:else}
				<div class="overflow-x-auto">
					<table class={table}>
						<thead>
							<tr>
								<th class={th}>Area</th>
								<th class={`${th} ${num}`}>Total</th>
								<th class={`${th} ${num}`}>Per 10 answers</th>
								<th class={`${th} ${num}`}>Recently per 10</th>
								<th class={th}>Compared with overall</th>
							</tr>
						</thead>
						<tbody>
							{#each stats.top_categories as c (c.category)}
								<tr>
									<td class={td}><strong>{categoryLabel(c.category)}</strong></td>
									<td class={`${td} ${num}`}>{c.total}</td>
									<td class={`${td} ${num}`}>{c.per_ten_answers}</td>
									<td class={`${td} ${num}`}>{c.recent_per_ten}</td>
									<td class={`${td} ${small} ${muted}`}>
										{#if stats.first_attempts_evaluated >= 20}{trendWord(c.per_ten_answers, c.recent_per_ten)}{:else}needs 20 answers{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<section class={card}>
			<h2 class="mb-2 text-xl font-bold">Recent results</h2>
			{#if stats.recent.length === 0}
				<p class={muted}>Nothing yet.</p>
			{:else}
				<p class={`${small} ${muted}`}>
					Your last {stats.recent.length} first attempts, oldest on the left. Taller means a better
					result (0 = not correct, 3 = correct). This is a practice measure, not a band score.
				</p>
				<div
					class="my-2 flex h-20 items-end gap-[3px]"
					role="img"
					aria-label="Result of each recent first attempt"
				>
					{#each stats.recent as r, i (i)}
						<div
							class="flex h-full flex-1 items-end"
							title="{verdictLabel[r.verdict]} ({typeLabel[r.type]})"
						>
							<div
								class="w-full rounded-t-[3px] bg-neutral-900 dark:bg-neutral-100"
								style="height: {((r.score + 1) / 4) * 100}%"
							></div>
						</div>
					{/each}
				</div>
				{#if stats.trend.enough}
					<p>
						Average of your last 10: <strong>{stats.trend.recent_avg}</strong> out of 3. The 10 before:
						<strong>{stats.trend.previous_avg}</strong>.
					</p>
				{:else}
					<p class={`${small} ${muted}`}>
						A comparison appears after 20 evaluated first attempts. You have
						{stats.first_attempts_evaluated}.
					</p>
				{/if}
			{/if}
		</section>

		<section class={card}>
			<h2 class="mb-2 text-xl font-bold">Rewriting after feedback</h2>
			{#if stats.rewrites.total === 0}
				<p class={muted}>
					You have not rewritten an answer yet. After feedback, try "Rewrite it in my own words". Producing
					the correct form yourself is what builds active English.
				</p>
			{:else}
				<p>
					You rewrote <strong>{stats.rewrites.total}</strong> answer{stats.rewrites.total === 1 ? '' : 's'}:
					<strong>{stats.rewrites.improved}</strong> got a better result,
					<strong>{stats.rewrites.same}</strong> stayed the same,
					<strong>{stats.rewrites.worse}</strong> got worse.
				</p>
			{/if}
		</section>
	{/if}
{/if}
