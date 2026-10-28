<script lang="ts">
	import type { Verdict } from '#lib/api';
	import { verdictLabel } from '#lib/labels';

	let { verdict }: { verdict: Verdict } = $props();
</script>

<!-- Shape and text carry the meaning, so it also works without colour. -->
<span class="badge {verdict}">{verdictLabel[verdict]}</span>

<style>
	.badge {
		display: inline-block;
		font-weight: 700;
		font-size: 0.85rem;
		padding: 0.2rem 0.75rem;
		border-radius: 999px;
		border: 1.5px solid var(--strong);
		white-space: nowrap;
	}
	.correct {
		background: var(--strong);
		color: var(--on-strong);
	}
	.mostly_correct {
		border-width: 2.5px;
	}
	.partially_correct {
		border-style: dashed;
	}
	.incorrect {
		border-style: dotted;
		border-width: 2.5px;
		color: var(--muted);
	}
</style>
