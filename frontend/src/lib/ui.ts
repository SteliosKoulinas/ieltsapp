import type { Verdict } from './api';

// Shared Tailwind class lists. Keeping them here avoids repeating the same
// long utility strings on every page. Every class stays fully written out so
// Tailwind's scanner can see it.

export const panel =
	'rounded-[10px] border border-neutral-300 bg-neutral-50 px-5 py-[1.1rem] dark:border-neutral-700 dark:bg-neutral-900';

export const card = `${panel} mb-4`;

export const muted = 'text-neutral-500 dark:text-neutral-400';

export const small = 'text-sm';

export const btn =
	'inline-block cursor-pointer rounded-full border-[1.5px] border-neutral-900 bg-transparent px-[1.1rem] py-[0.6rem] font-semibold leading-tight text-neutral-900 no-underline hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-45 dark:border-neutral-100 dark:text-neutral-100 dark:hover:bg-neutral-900';

export const btnPrimary = `${btn} bg-neutral-900 text-white hover:bg-neutral-900 hover:opacity-90 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-100`;

export const alert =
	'mb-4 rounded-[10px] border-[1.5px] border-neutral-900 px-[1.1rem] py-[0.9rem] dark:border-neutral-100';

export const alertTitle = 'mb-[0.15rem] block';

export const narrow = 'max-w-2xl';

export const answerText = 'whitespace-pre-wrap break-words text-[1.05rem]';

export const textarea =
	'w-full resize-y rounded-[10px] border-[1.5px] border-neutral-300 bg-white px-[0.9rem] py-[0.8rem] font-sans text-base leading-[1.55] text-neutral-900 focus-visible:border-neutral-900 disabled:opacity-60 dark:border-neutral-700 dark:bg-neutral-950 dark:text-neutral-100 dark:focus-visible:border-neutral-100';

export const table = 'w-full border-collapse';

export const th =
	'border-b border-neutral-300 px-2 py-[0.55rem] text-left align-top text-xs font-bold uppercase tracking-[0.05em] text-neutral-500 dark:border-neutral-700 dark:text-neutral-400';

export const td =
	'border-b border-neutral-300 px-2 py-[0.55rem] text-left align-top dark:border-neutral-700';

export const num = 'text-right tabular-nums';

// Shape and text carry the verdict, so colour is never the only signal.
export const verdictPattern: Record<Verdict, string> = {
	correct: 'bg-neutral-900 dark:bg-neutral-100',
	mostly_correct:
		'bg-[repeating-linear-gradient(45deg,currentColor_0_3px,transparent_3px_6px)]',
	partially_correct:
		'bg-[repeating-linear-gradient(90deg,currentColor_0_2px,transparent_2px_6px)]',
	incorrect: 'bg-white dark:bg-neutral-950'
};

export const verdictBadge =
	'inline-block whitespace-nowrap rounded-full border-[1.5px] border-neutral-900 px-3 py-[0.2rem] text-[0.85rem] font-bold dark:border-neutral-100';

export const verdictBadgeStyle: Record<Verdict, string> = {
	correct: 'bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900',
	mostly_correct: 'border-[2.5px]',
	partially_correct: 'border-dashed',
	incorrect: 'border-[2.5px] border-dotted text-neutral-500 dark:text-neutral-400'
};
