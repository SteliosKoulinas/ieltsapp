export function formatDateTime(iso: string): string {
	return new Date(iso).toLocaleString(undefined, {
		day: 'numeric',
		month: 'short',
		year: 'numeric',
		hour: '2-digit',
		minute: '2-digit'
	});
}

/** "2026-10-27" to a short weekday such as "Tue". */
export function weekday(day: string): string {
	const [y, m, d] = day.split('-').map(Number);
	return new Date(y, m - 1, d).toLocaleDateString(undefined, { weekday: 'short' });
}

export function dayMonth(day: string): string {
	const [y, m, d] = day.split('-').map(Number);
	return new Date(y, m - 1, d).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
}

export function percent(part: number, total: number): number {
	return total === 0 ? 0 : Math.round((part / total) * 100);
}

/** Milliseconds as "12.4s" or "1m 05s". */
export function formatDuration(ms: number): string {
	const seconds = Math.max(0, ms) / 1000;
	if (seconds < 60) return `${seconds.toFixed(1)}s`;
	const minutes = Math.floor(seconds / 60);
	const rest = Math.round(seconds % 60);
	return `${minutes}m ${rest.toString().padStart(2, '0')}s`;
}

export function userTimeZone(): string {
	try {
		return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
	} catch {
		return 'UTC';
	}
}
