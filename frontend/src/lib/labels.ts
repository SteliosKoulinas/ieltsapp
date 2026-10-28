import type { ExerciseType, Verdict } from './api';

export const verdictLabel: Record<Verdict, string> = {
	correct: 'Correct',
	mostly_correct: 'Mostly correct',
	partially_correct: 'Partly correct',
	incorrect: 'Not correct'
};

export const verdictOrder: Verdict[] = ['correct', 'mostly_correct', 'partially_correct', 'incorrect'];

export const typeLabel: Record<ExerciseType, string> = {
	vocab: 'Vocabulary in context',
	writing: 'Short writing'
};

const categories: Record<string, string> = {
	articles: 'Articles',
	verb_tense: 'Verb tense',
	subject_verb_agreement: 'Subject-verb agreement',
	prepositions: 'Prepositions',
	missing_subject: 'Missing subject',
	word_choice: 'Word choice',
	collocations: 'Collocations',
	sentence_structure: 'Sentence structure',
	linking: 'Linking ideas',
	clarity: 'Clarity',
	vocabulary_meaning: 'Word meaning',
	noun_number: 'Singular and plural'
};

export function categoryLabel(id: string): string {
	return categories[id] ?? id.replaceAll('_', ' ');
}

export const severityLabel: Record<string, string> = {
	error: 'Mistake',
	unnatural: 'Sounds unnatural',
	style: 'Optional improvement'
};
