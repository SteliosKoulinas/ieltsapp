// Browser speech for listening dictation.
//
// The quality of the Web Speech API depends entirely on the voices installed on
// the device. Instead of using the browser default (which is often a compact,
// robotic voice), we rank the available voices and pick the most natural
// English one: neural/enhanced voices first, network voices over local compact
// ones, and a British accent before other accents for IELTS.
//
// Nothing is downloaded from a third party. If the device has good voices, this
// uses them; otherwise it falls back to the best of what is there.

export interface VoiceInfo {
	uri: string;
	name: string;
	lang: string;
	local: boolean;
	natural: boolean;
}

// IELTS is British, so a British voice is preferred. The others are still quite
// usable and are ranked below it rather than excluded.
const accentRank: Record<string, number> = {
	'en-gb': 40,
	'en-us': 32,
	'en-au': 26,
	'en-nz': 25,
	'en-ie': 24,
	'en-ca': 20,
	'en-za': 16,
	'en-in': 12
};

// Names that are known to be high quality on the platforms people use.
const naturalNames =
	/sonia|libby|ryan|aria|jenny|guy|emma|brian|ava|andrew|maisie|thomas|oliver|samantha|daniel|serena|kate|arthur|moira|tessa|fiona|karen|lee|google/;

// Novelty and screen-reader voices. These are either silly or clearly robotic.
const badNames =
	/compact|espeak|festival|pico|bot|robot|novelty|zarvox|albert|bad news|bells|bubbles|cellos|wobble|boing|organ|trinoids|whisper|good news|jester|superstar|bahh|deranged|hysterical|pipe organ|grandma|grandpa|rocko|shelley|sandy|flo|eddy|reed|junior|ralph|fred|kathy|albert|zarvox|shelley/;

function normaliseLang(lang: string): string {
	return lang.replace('_', '-').toLowerCase();
}

function scoreVoice(v: SpeechSynthesisVoice): number {
	const lang = normaliseLang(v.lang);
	if (!lang.startsWith('en')) return -1000;
	const name = v.name.toLowerCase();
	let score = accentRank[lang] ?? 8;

	if (/natural|neural/.test(name)) score += 60;
	if (/premium|enhanced/.test(name)) score += 50;
	if (/siri/.test(name)) score += 45;
	if (/^google/.test(name)) score += 35;
	if (naturalNames.test(name)) score += 30;
	// Network voices on Chrome and Edge are usually much better than compact
	// local ones, but an offline natural voice (Siri, enhanced) beats them.
	if (!v.localService && !/siri|premium|enhanced/.test(name)) score += 20;
	if (badNames.test(name)) score -= 120;
	// A compact local voice is the classic robotic one.
	if (/compact/.test(name)) score -= 40;
	return score;
}

export function speechSupported(): boolean {
	return typeof window !== 'undefined' && 'speechSynthesis' in window;
}

// Chrome populates getVoices() asynchronously. Waiting for voiceschanged is
// what makes the first recording sound right instead of falling back to the
// system default.
export function loadVoices(timeoutMs = 2500): Promise<SpeechSynthesisVoice[]> {
	if (!speechSupported()) return Promise.resolve([]);
	const synth = window.speechSynthesis;
	const available = synth.getVoices();
	if (available.length > 0) return Promise.resolve(available);

	return new Promise((resolve) => {
		let done = false;
		const finish = () => {
			if (done) return;
			done = true;
			synth.removeEventListener('voiceschanged', finish);
			resolve(synth.getVoices());
		};
		synth.addEventListener('voiceschanged', finish);
		window.setTimeout(finish, timeoutMs);
	});
}

export function rankVoices(voices: SpeechSynthesisVoice[]): SpeechSynthesisVoice[] {
	return [...voices]
		.filter((v) => normaliseLang(v.lang).startsWith('en'))
		.sort((a, b) => scoreVoice(b) - scoreVoice(a) || a.name.localeCompare(b.name));
}

export function bestVoice(voices: SpeechSynthesisVoice[]): SpeechSynthesisVoice | null {
	return rankVoices(voices)[0] ?? null;
}

export function describeVoice(v: SpeechSynthesisVoice | null): VoiceInfo | null {
	if (!v) return null;
	return {
		uri: v.voiceURI,
		name: v.name,
		lang: v.lang,
		local: v.localService,
		natural: /natural|neural|premium|enhanced|siri/i.test(v.name)
	};
}

export interface SpeakOptions {
	voice?: SpeechSynthesisVoice | null;
	rate?: number;
	pitch?: number;
	volume?: number;
	lang?: string;
}

// speakGeneration invalidates a queued utterance when playback is cancelled or
// replaced, so a delayed speak never starts after the learner has moved on.
let speakGeneration = 0;

// Speaks one recording and resolves when it finishes. A watchdog resolves the
// promise on engines that never fire onend after a cancel, so the UI cannot get
// stuck showing "Speaking…".
export function speak(text: string, opts: SpeakOptions = {}): Promise<void> {
	if (!speechSupported()) return Promise.reject(new Error('Speech is not available in this browser.'));
	const synth = window.speechSynthesis;
	if (!text.trim()) return Promise.resolve();

	// Cancel anything still playing. Chrome leaves the queue paused after some
	// cancels, so explicitly resume.
	synth.cancel();
	const generation = ++speakGeneration;
	if (synth.paused) synth.resume();

	const utterance = new SpeechSynthesisUtterance(text);
	if (opts.voice) {
		utterance.voice = opts.voice;
		utterance.lang = opts.voice.lang;
	} else if (opts.lang) {
		utterance.lang = opts.lang;
	}
	utterance.rate = clamp(opts.rate ?? 0.85, 0.5, 1.5);
	utterance.pitch = clamp(opts.pitch ?? 1, 0.5, 2);
	utterance.volume = clamp(opts.volume ?? 1, 0, 1);

	return new Promise((resolve, reject) => {
		let settled = false;
		const finish = () => {
			if (settled) return;
			settled = true;
			window.clearTimeout(watchdog);
			resolve();
		};
		utterance.onend = finish;
		utterance.onerror = (event) => {
			if (settled) return;
			// A cancelled/interrupted utterance is not a failure from the
			// learner's point of view.
			if (event.error === 'interrupted' || event.error === 'canceled') {
				finish();
				return;
			}
			settled = true;
			window.clearTimeout(watchdog);
			reject(new Error(event.error || 'Speech failed.'));
		};
		// Generous estimate: real speech is faster than 8 characters a second.
		const watchdog = window.setTimeout(finish, (text.length / 8) * 1000 / utterance.rate + 6000);
		// Chrome sometimes drops an utterance that is queued in the same tick as
		// cancel(). A short delay makes the replay reliable. If playback was
		// cancelled in the meantime, do not start it.
		window.setTimeout(() => {
			if (generation !== speakGeneration) {
				finish();
				return;
			}
			synth.speak(utterance);
		}, 50);
	});
}

export function cancelSpeech(): void {
	if (!speechSupported()) return;
	speakGeneration += 1;
	window.speechSynthesis.cancel();
}

function clamp(n: number, lo: number, hi: number): number {
	return Math.min(hi, Math.max(lo, n));
}

// The rates offered in the UI. The default is deliberately slower than the
// browser default, which learners found too fast to follow.
export const speechRates = [
	{ value: 0.65, label: 'Slow' },
	{ value: 0.8, label: 'Natural' },
	{ value: 1, label: 'Exam speed' }
] as const;

export const defaultSpeechRate = 0.8;

export const kindLabel: Record<string, string> = {
	word: 'Word',
	phrase: 'Phrase',
	sentence: 'Sentence',
	phone: 'Phone number',
	address: 'Address',
	price: 'Price',
	date: 'Date',
	time: 'Time',
	postcode: 'Postcode',
	reference: 'Reference number',
	quantity: 'Quantity',
	percentage: 'Percentage',
	distance: 'Distance',
	weight: 'Weight',
	duration: 'Duration',
	room: 'Room number'
};
