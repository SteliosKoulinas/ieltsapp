<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, type Stats, type Verdict } from '#lib/api';
	import { categoryLabel, typeLabel, verdictLabel, verdictOrder } from '#lib/labels';
	import { dayMonth, formatDateTime, percent, userTimeZone, weekday } from '#lib/format';

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

<h1>Statistics</h1>

{#if loading}
	<div class="card" aria-busy="true"><p class="muted">Loading…</p></div>
{:else if error}
	<div class="alert" role="alert"><strong>Problem</strong><span>{error}</span></div>
{:else if stats}
	{#if stats.totals.first_attempts === 0}
		<div class="card">
			<p>There is nothing to show yet. Answer a few exercises and your statistics will appear here.</p>
			<a class="btn primary" href="/">Start practising</a>
		</div>
	{:else}
		<div class="cards">
			<div class="card stat">
				<span class="big">{stats.totals.evaluated}</span>
				<span class="muted small">answers evaluated</span>
			</div>
			<div class="card stat">
				<span class="big">{stats.totals.first_attempts}</span>
				<span class="muted small">exercises attempted</span>
			</div>
			<div class="card stat">
				<span class="big">{stats.totals.rewrites}</span>
				<span class="muted small">rewrites</span>
			</div>
			<div class="card stat">
				<span class="big">{stats.streak_days}</span>
				<span class="muted small">day{stats.streak_days === 1 ? '' : 's'} in a row</span>
			</div>
		</div>
		{#if stats.totals.failed > 0}
			<p class="small muted">
				{stats.totals.failed} answer{stats.totals.failed === 1 ? ' was' : 's were'} saved but not evaluated. Open
				them from <a href="/history">History</a> to evaluate again. They are not counted below.
			</p>
		{/if}

		{#if stats.first_attempts_evaluated < 10}
			<div class="alert">
				<strong>Only {stats.first_attempts_evaluated} evaluated answer{stats.first_attempts_evaluated === 1 ? '' : 's'} so far</strong>
				<span>Treat the numbers below as a first impression, not a conclusion. They become more meaningful after about 10 to 20 answers.</span>
			</div>
		{/if}

		<section class="card">
			<h2>Vocabulary schedule</h2>
			<p class="small muted">
				Words return after a waiting time that grows each time you answer them correctly. A wrong
				answer brings the word back the same day. Due reviews always come first and are never
				limited; only new words are capped.
			</p>
			{#if stats.srs.tracked === 0}
				<p class="muted">No words are in the schedule yet. Answer a vocabulary exercise to start.</p>
			{:else}
				<div class="cards">
					<div class="card stat">
						<span class="big">{stats.srs.due_now}</span>
						<span class="muted small">due now</span>
					</div>
					<div class="card stat">
						<span class="big">{stats.srs.new_today}{stats.srs.new_per_day > 0 ? ` / ${stats.srs.new_per_day}` : ''}</span>
						<span class="muted small">new today{stats.srs.new_per_day > 0 ? '' : ' (no limit)'}</span>
					</div>
					<div class="card stat">
						<span class="big">{stats.srs.tracked}</span>
						<span class="muted small">words tracked</span>
					</div>
					<div class="card stat">
						<span class="big">{stats.srs.mastered}</span>
						<span class="muted small">in the last box</span>
					</div>
				</div>
				<div class="scroll">
					<table>
						<thead>
							<tr>
								<th>Box</th>
								<th class="num">Words</th>
								<th>Next review</th>
							</tr>
						</thead>
						<tbody>
							{#each stats.srs.boxes as b (b.box)}
								<tr>
									<td>{b.box}</td>
									<td class="num">{b.count}</td>
									<td class="muted small">
										{#if b.box === 1}10 minutes
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
					<p class="small muted">Next review: {formatDateTime(stats.srs.next_due_at)}.</p>
				{/if}
				<p class="small muted">
					A correct or mostly correct answer moves the word up one box and at least 1 day away.
					A wrong or partial answer moves it back to box 1, 10 minutes away.
				</p>
			{/if}
		</section>

		<section class="card">
			<h2>How your first attempts went</h2>
			<p class="small muted">
				Only the first answer to each exercise counts here, so rewrites made after feedback do not
				inflate the picture.
			</p>
			{#if stats.first_attempts_evaluated === 0}
				<p class="muted">No evaluated first attempts yet.</p>
			{:else}
				<div class="stack" role="img" aria-label="Share of results by verdict">
					{#each verdictOrder as v (v)}
						{#if overall[v] > 0}
							<div class="seg {v}" style="flex: {overall[v]}" title="{verdictLabel[v]}: {overall[v]}"></div>
						{/if}
					{/each}
				</div>
				<ul class="legend">
					{#each verdictOrder as v (v)}
						<li>
							<span class="swatch {v}" aria-hidden="true"></span>
							{verdictLabel[v]}: <strong>{overall[v]}</strong>
							<span class="muted">({percent(overall[v], stats.first_attempts_evaluated)}%)</span>
						</li>
					{/each}
				</ul>

				{#if stats.by_type.length > 0}
					<table>
						<thead>
							<tr>
								<th>Exercise type</th>
								<th class="num">Answers</th>
								{#each verdictOrder as v (v)}<th class="num">{verdictLabel[v]}</th>{/each}
							</tr>
						</thead>
						<tbody>
							{#each stats.by_type as t (t.type)}
								<tr>
									<td>{typeLabel[t.type]}</td>
									<td class="num">{t.total}</td>
									{#each verdictOrder as v (v)}<td class="num">{t.verdicts[v] ?? 0}</td>{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			{/if}
		</section>

		<section class="card">
			<h2>Practice in the last 14 days</h2>
			<p class="small muted">
				{answersLast14} answer{answersLast14 === 1 ? '' : 's'} on {activeDays} of 14 days.
			</p>
			<div class="bars" role="img" aria-label="Answers evaluated per day, last 14 days">
				{#each stats.daily as d (d.date)}
					<div class="bar-col" title="{dayMonth(d.date)}: {d.count}">
						<span class="count small">{d.count || ''}</span>
						<div class="bar" style="height: {(d.count / maxDaily) * 100}%"></div>
						<span class="day small muted">{weekday(d.date)}</span>
					</div>
				{/each}
			</div>
		</section>

		<section class="card">
			<h2>Your most frequent mistakes</h2>
			<p class="small muted">
				Only items marked as real mistakes in first attempts are counted, as a rate per 10 answers so
				the numbers do not depend on how much you wrote. "Recently" means your last
				{stats.category_window} first attempts.
			</p>
			{#if stats.top_categories.length === 0}
				<p class="muted">No mistakes recorded yet.</p>
			{:else}
				<div class="scroll">
					<table>
						<thead>
							<tr>
								<th>Area</th>
								<th class="num">Total</th>
								<th class="num">Per 10 answers</th>
								<th class="num">Recently per 10</th>
								<th>Compared with overall</th>
							</tr>
						</thead>
						<tbody>
							{#each stats.top_categories as c (c.category)}
								<tr>
									<td><strong>{categoryLabel(c.category)}</strong></td>
									<td class="num">{c.total}</td>
									<td class="num">{c.per_ten_answers}</td>
									<td class="num">{c.recent_per_ten}</td>
									<td class="muted small">
										{#if stats.first_attempts_evaluated >= 20}{trendWord(c.per_ten_answers, c.recent_per_ten)}{:else}needs 20 answers{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<section class="card">
			<h2>Recent results</h2>
			{#if stats.recent.length === 0}
				<p class="muted">Nothing yet.</p>
			{:else}
				<p class="small muted">
					Your last {stats.recent.length} first attempts, oldest on the left. Taller means a better
					result (0 = not correct, 3 = correct). This is a practice measure, not a band score.
				</p>
				<div class="spark" role="img" aria-label="Result of each recent first attempt">
					{#each stats.recent as r, i (i)}
						<div class="spark-col" title="{verdictLabel[r.verdict]} ({typeLabel[r.type]})">
							<div class="spark-bar" style="height: {((r.score + 1) / 4) * 100}%"></div>
						</div>
					{/each}
				</div>
				{#if stats.trend.enough}
					<p>
						Average of your last 10: <strong>{stats.trend.recent_avg}</strong> out of 3. The 10 before:
						<strong>{stats.trend.previous_avg}</strong>.
					</p>
				{:else}
					<p class="small muted">
						A comparison appears after 20 evaluated first attempts. You have
						{stats.first_attempts_evaluated}.
					</p>
				{/if}
			{/if}
		</section>

		<section class="card">
			<h2>Rewriting after feedback</h2>
			{#if stats.rewrites.total === 0}
				<p class="muted">
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

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}
	.stat {
		display: flex;
		flex-direction: column;
		margin: 0;
	}
	.big {
		font-size: 2rem;
		font-weight: 800;
		line-height: 1.1;
	}

	/* Verdict colours are shades of one ink, differentiated by pattern too. */
	.stack {
		display: flex;
		height: 1.6rem;
		border: 1.5px solid var(--strong);
		border-radius: 999px;
		overflow: hidden;
		margin: 0.5rem 0 0.75rem;
	}
	.seg.correct,
	.swatch.correct {
		background: var(--strong);
	}
	.seg.mostly_correct,
	.swatch.mostly_correct {
		background: repeating-linear-gradient(45deg, var(--strong) 0 3px, var(--bg) 3px 6px);
	}
	.seg.partially_correct,
	.swatch.partially_correct {
		background: repeating-linear-gradient(90deg, var(--strong) 0 2px, var(--bg) 2px 6px);
	}
	.seg.incorrect,
	.swatch.incorrect {
		background: var(--bg);
	}
	.legend {
		list-style: none;
		margin: 0 0 1rem;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 1.25rem;
	}
	.swatch {
		display: inline-block;
		width: 0.9rem;
		height: 0.9rem;
		border: 1.5px solid var(--strong);
		border-radius: 3px;
		vertical-align: -0.1em;
		margin-right: 0.25rem;
	}

	.bars {
		display: flex;
		align-items: flex-end;
		gap: 0.35rem;
		height: 9rem;
		margin-top: 0.75rem;
	}
	.bar-col {
		flex: 1;
		min-width: 0;
		height: 100%;
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		align-items: stretch;
		text-align: center;
	}
	.bar {
		background: var(--strong);
		border-radius: 4px 4px 0 0;
		min-height: 2px;
	}
	.count {
		min-height: 1.3em;
	}
	.day {
		margin-top: 0.2rem;
	}

	.spark {
		display: flex;
		align-items: flex-end;
		gap: 3px;
		height: 5rem;
		margin: 0.5rem 0 0.75rem;
	}
	.spark-col {
		flex: 1;
		height: 100%;
		display: flex;
		align-items: flex-end;
	}
	.spark-bar {
		width: 100%;
		background: var(--strong);
		border-radius: 3px 3px 0 0;
	}
	.scroll {
		overflow-x: auto;
	}
</style>
