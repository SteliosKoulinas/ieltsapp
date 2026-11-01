// Typed client for the Go API. All requests go to the same origin under /api:
// nginx proxies it in production and the Vite dev server proxies it in development.

export type ExerciseType = 'vocab' | 'writing';
export type Verdict = 'correct' | 'mostly_correct' | 'partially_correct' | 'incorrect';

export interface Exercise {
	id: number;
	type: ExerciseType;
	context?: string;
	prompt: string;
	target_word?: string;
	expected_length?: string;
	criteria?: string[];
	topic: string;
	cefr: string;
	// Vocabulary only: "review" is a scheduled repetition, "new" a first exposure.
	mode?: 'review' | 'new';
	due_at?: string;
}

export interface Issue {
	category: string;
	severity: 'error' | 'unnatural' | 'style';
	original: string;
	correction: string;
	explanation: string;
}

export interface Evaluation {
	verdict: Verdict;
	summary: string;
	strengths: string[];
	issues: Issue[];
	improved_version: string;
	confidence: 'high' | 'medium' | 'low';
}

export interface AttemptResult {
	attempt_id: string;
	exercise_id: number;
	status: string;
	evaluation: Evaluation;
	reused: boolean;
}

export interface AttemptDetail {
	id: string;
	exercise: Exercise;
	reference_meaning?: string;
	parent_attempt_id: string | null;
	answer: string;
	status: 'pending' | 'evaluated' | 'failed';
	evaluation?: Evaluation;
	failure_code?: string;
	created_at: string;
}

export interface AttemptSummary {
	id: string;
	created_at: string;
	status: 'pending' | 'evaluated' | 'failed';
	verdict?: Verdict;
	is_rewrite: boolean;
	exercise_id: number;
	type: ExerciseType;
	target_word?: string;
	prompt: string;
	answer: string;
}

export interface AttemptList {
	items: AttemptSummary[];
	total: number;
	limit: number;
	offset: number;
}

export interface Stats {
	totals: { first_attempts: number; rewrites: number; evaluated: number; failed: number };
	first_attempts_evaluated: number;
	by_type: { type: ExerciseType; total: number; verdicts: Partial<Record<Verdict, number>> }[];
	daily: { date: string; count: number }[];
	streak_days: number;
	top_categories: {
		category: string;
		total: number;
		per_ten_answers: number;
		recent_total: number;
		recent_per_ten: number;
	}[];
	category_window: number;
	recent: { at: string; type: ExerciseType; verdict: Verdict; score: number }[];
	trend: { enough: boolean; recent_avg: number; previous_avg: number };
	rewrites: { total: number; improved: number; same: number; worse: number };
	srs: {
		tracked: number;
		due_now: number;
		new_today: number;
		new_per_day: number;
		boxes: { box: number; count: number }[];
		mastered: number;
		next_due_at?: string;
	};
}

export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		message: string,
		public retryable = false
	) {
		super(message);
	}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	let res: Response;
	try {
		res = await fetch('/api' + path, {
			...init,
			headers: { 'Content-Type': 'application/json', ...init?.headers }
		});
	} catch {
		throw new ApiError(0, 'network', 'Cannot reach the server. Check that the app is running.', true);
	}
	let body: unknown = null;
	try {
		body = await res.json();
	} catch {
		/* not JSON */
	}
	if (!res.ok) {
		const b = (body ?? {}) as { error?: string; message?: string; retryable?: boolean };
		throw new ApiError(
			res.status,
			b.error ?? 'error',
			b.message ?? `The server answered with status ${res.status}.`,
			b.retryable ?? false
		);
	}
	return body as T;
}

export const api = {
	nextExercise(type: 'any' | ExerciseType, excludeId?: number, tz?: string, unseen?: boolean) {
		const q = new URLSearchParams({ type });
		if (excludeId) q.set('exclude', String(excludeId));
		if (tz) q.set('tz', tz);
		if (unseen) q.set('unseen', '1');
		return request<Exercise>(`/exercises/next?${q}`);
	},
	submitAttempt(body: {
		id: string;
		exercise_id: number;
		answer: string;
		parent_attempt_id?: string | null;
	}) {
		return request<AttemptResult>('/attempts', { method: 'POST', body: JSON.stringify(body) });
	},
	listAttempts(limit: number, offset: number) {
		return request<AttemptList>(`/attempts?limit=${limit}&offset=${offset}`);
	},
	getAttempt(id: string) {
		return request<AttemptDetail>(`/attempts/${encodeURIComponent(id)}`);
	},
	stats(tz: string) {
		return request<Stats>(`/stats?tz=${encodeURIComponent(tz)}`);
	}
};
