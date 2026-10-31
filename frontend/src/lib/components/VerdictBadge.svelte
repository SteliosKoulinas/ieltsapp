<script lang="ts">
	import type { Verdict } from '#lib/api';
	import { verdictLabel } from '#lib/labels';
	import { verdictBadge, verdictBadgeStyle } from '#lib/ui';

	let { verdict }: { verdict: Verdict } = $props();
</script>

<!-- Shape and text carry the meaning, so it also works without colour. -->
<span class={`${verdictBadge} ${verdictBadgeStyle[verdict]}`}>{verdictLabel[verdict]}</span>
