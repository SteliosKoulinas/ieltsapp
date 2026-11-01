// Package generate creates new practice material on demand. It is called only
// by the manual "api generate" command, never by the server or a timer.
//
// The model output is treated as untrusted: every item is validated with the
// same rules as the built-in seed, duplicate slugs and duplicate content are
// dropped, and only then is a batch file written and inserted.
package generate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ielts/backend/internal/eval"
	"ielts/backend/internal/store"
)

// LLM is the small part of the Gemini client this package needs.
type LLM interface {
	Generate(ctx context.Context, system, user string) (string, error)
}

type Options struct {
	Type   string     // vocab, writing or listening
	Count  int        // number of new items to keep
	OutDir string     // directory for the batch file
	Words  []WordSpec // optional fixed vocabulary targets; Count is ignored
}

// WordSpec is one requested vocabulary target. Tier and CEFR are curriculum
// hints from the operator; zero or empty means the model chooses.
type WordSpec struct {
	Word string
	Tier int
	CEFR string
}

// Result reports what happened, including the batch path when one was written.
type Result struct {
	Path      string
	Requested int
	Generated int
	Accepted  int
	Skipped   int
	Inserted  int
	Missing   []string // requested words the model did not deliver
}

// Run asks the model for new items, validates them, writes a batch file and
// inserts it. If insertion fails, the file is left on disk so it can be loaded
// later without paying for the same generation again.
func Run(ctx context.Context, llm LLM, st *store.Store, opts Options) (*Result, error) {
	if opts.Type != eval.TypeVocab && opts.Type != eval.TypeWriting && opts.Type != eval.TypeListening {
		return nil, fmt.Errorf("type must be vocab, writing or listening")
	}
	wordsMode := len(opts.Words) > 0
	if wordsMode {
		if opts.Type != eval.TypeVocab {
			return nil, fmt.Errorf("a fixed word list is only supported for vocab")
		}
		if len(opts.Words) > 30 {
			return nil, fmt.Errorf("at most 30 words can be requested per batch")
		}
		opts.Count = len(opts.Words)
	}
	if opts.Count < 1 || opts.Count > 30 {
		return nil, fmt.Errorf("count must be between 1 and 30")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, fmt.Errorf("output directory is required")
	}
	for _, spec := range opts.Words {
		if strings.TrimSpace(spec.Word) == "" {
			return nil, fmt.Errorf("word list contains an empty word")
		}
		if spec.Tier < 0 || spec.Tier > 4 {
			return nil, fmt.Errorf("word %q has an invalid tier", spec.Word)
		}
	}

	idx, err := st.ContentIndex(ctx, opts.Type)
	if err != nil {
		return nil, fmt.Errorf("read existing content: %w", err)
	}
	ask := min(opts.Count+3, 60)
	if wordsMode {
		ask = opts.Count
	}
	raw, err := llm.Generate(ctx, systemPrompt(opts.Type, wordsMode), userPrompt(opts.Type, ask, idx, opts.Words))
	if err != nil {
		return nil, err
	}
	items, err := parseItems(raw)
	if err != nil {
		return nil, err
	}

	res := &Result{Requested: opts.Count, Generated: len(items)}
	accepted := make([]json.RawMessage, 0, opts.Count)
	seenSlugs := map[string]bool{}
	seenKeys := map[string]bool{}
	requested := make(map[string]WordSpec, len(opts.Words))
	for _, spec := range opts.Words {
		requested[normalizeWord(spec.Word)] = spec
	}
	covered := map[string]bool{}
	for _, item := range items {
		if len(accepted) >= opts.Count {
			break
		}
		normalized, entry, err := normalizeItem(item, opts.Type)
		if err != nil {
			res.Skipped++
			continue
		}
		if wordsMode {
			key := normalizeWord(entry.targetWord)
			spec, ok := requested[key]
			if !ok || covered[key] {
				res.Skipped++
				continue
			}
			if !containsExactWord(entry.context, spec.Word) {
				res.Skipped++
				continue
			}
			normalized, err = applyWordSpec(normalized, &entry, spec)
			if err != nil {
				res.Skipped++
				continue
			}
		}
		if idx.Slugs[entry.slug] || seenSlugs[entry.slug] {
			res.Skipped++
			continue
		}
		key := contentKey(entry)
		if key == "" || idx.Keys[key] || seenKeys[key] {
			res.Skipped++
			continue
		}
		if err := store.ValidateSeedJSON([]byte("["+string(normalized)+"]"), "generated "+entry.slug); err != nil {
			res.Skipped++
			continue
		}
		seenSlugs[entry.slug] = true
		seenKeys[key] = true
		accepted = append(accepted, normalized)
		if wordsMode {
			covered[normalizeWord(entry.targetWord)] = true
		}
	}
	res.Accepted = len(accepted)
	res.Skipped = res.Generated - res.Accepted
	if wordsMode {
		for _, spec := range opts.Words {
			if !covered[normalizeWord(spec.Word)] {
				res.Missing = append(res.Missing, spec.Word)
			}
		}
	}
	if res.Accepted == 0 {
		return res, errors.New("the model returned no new valid items; nothing was written")
	}

	path, err := writeBatch(opts.OutDir, opts.Type, accepted)
	res.Path = path
	if err != nil {
		return res, err
	}
	inserted, err := st.SeedFile(ctx, path)
	res.Inserted = inserted
	if err != nil {
		return res, fmt.Errorf("batch written to %s but insertion failed: %w", path, err)
	}
	return res, nil
}

type entry struct {
	slug       string
	typ        string
	targetWord string
	prompt     string
	audioText  string
	context    string
}

func normalizeItem(raw json.RawMessage, typ string) (json.RawMessage, entry, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, entry{}, err
	}
	// These two fields are provenance, not creative choices. The command owns
	// them so a model cannot label generated material as original or as another
	// exercise type.
	m["type"] = typ
	m["source"] = "generated"
	// Models occasionally pad a field with a stray space. It is invisible in
	// JSON but shows up in the exercise, so trim the text fields here.
	for _, key := range []string{
		"slug", "target_word", "context", "intended_meaning", "prompt",
		"topic", "expected_length", "audio_text", "answer_text",
	} {
		if s, ok := m[key].(string); ok {
			m[key] = strings.TrimSpace(s)
		}
	}
	if _, ok := m["accepted_forms"]; !ok {
		m["accepted_forms"] = []string{}
	}
	if _, ok := m["criteria"]; !ok && typ == eval.TypeWriting {
		m["criteria"] = []string{}
	}
	normalized, err := json.Marshal(m)
	if err != nil {
		return nil, entry{}, err
	}
	e := entry{
		slug:       stringField(m, "slug"),
		typ:        typ,
		targetWord: stringField(m, "target_word"),
		prompt:     stringField(m, "prompt"),
		audioText:  stringField(m, "audio_text"),
		context:    stringField(m, "context"),
	}
	prefix := map[string]string{
		eval.TypeVocab: "vocab-", eval.TypeWriting: "writing-", eval.TypeListening: "listen-",
	}[typ]
	if !strings.HasPrefix(e.slug, prefix) {
		return nil, entry{}, fmt.Errorf("slug %q must start with %s", e.slug, prefix)
	}
	return normalized, e, nil
}

// normalizeWord gives a stable key for a requested target. Case and surrounding
// space are the only differences the generator treats as the same word.
func normalizeWord(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// containsExactWord reports whether text contains word as a whole word. The
// store validator uses a substring check, which would accept "formulate" for
// "form"; generated vocabulary should use the exact target instead.
func containsExactWord(text, word string) bool {
	word = strings.TrimSpace(word)
	if word == "" {
		return false
	}
	pattern := `(?i)(^|[^a-z0-9])` + regexp.QuoteMeta(word) + `([^a-z0-9]|$)`
	return regexp.MustCompile(pattern).MatchString(text)
}

// applyWordSpec replaces the model's tier and CEFR with the operator's values,
// and gives the item a deterministic slug and target spelling. The slug can no
// longer collide with another word in the list, and the stored target is the
// exact form the context was checked against.
func applyWordSpec(raw json.RawMessage, e *entry, spec WordSpec) (json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	slug := "vocab-" + slugifyWord(spec.Word)
	m["slug"] = slug
	m["target_word"] = spec.Word
	if spec.Tier > 0 {
		m["tier"] = spec.Tier
	}
	if spec.CEFR != "" {
		m["cefr"] = spec.CEFR
	}
	e.slug = slug
	e.targetWord = spec.Word
	return json.Marshal(m)
}

func slugifyWord(word string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(word)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case r == '\'':
			// Drop apostrophes rather than turning them into a separator.
		case r == '-' || r == ' ':
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func contentKey(e entry) string {
	var key string
	switch e.typ {
	case eval.TypeVocab:
		key = e.targetWord
	case eval.TypeWriting:
		key = e.prompt
	case eval.TypeListening:
		key = e.audioText
	}
	return strings.ToLower(strings.TrimSpace(key))
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func parseItems(raw string) ([]json.RawMessage, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var items []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		return items, nil
	}
	// Some models wrap the array in an object such as {"items": [...]}.
	var wrapper struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil && wrapper.Items != nil {
		return wrapper.Items, nil
	}
	// Last resort: take the outermost array from surrounding commentary.
	start, end := strings.Index(raw, "["), strings.LastIndex(raw, "]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(raw[start:end+1]), &items); err == nil {
			return items, nil
		}
	}
	return nil, errors.New("the model response was not a JSON array of items")
}

func writeBatch(dir, typ string, items []json.RawMessage) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	name := fmt.Sprintf("%s-%s.json", time.Now().UTC().Format("20060102-150405"), typ)
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		name = fmt.Sprintf("%s-%s-%d.json", time.Now().UTC().Format("20060102-150405"), typ, os.Getpid())
		path = filepath.Join(dir, name)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func systemPrompt(typ string, fixedTargets bool) string {
	common := `You generate original IELTS Academic practice material for one learner. This is not official IELTS content and must never be described as official. Return ONLY a JSON array of objects. No markdown, no commentary, no wrapper object.

Every object must have these fields: slug, type, prompt, topic, cefr, tier, source.
- slug: lowercase ASCII letters, digits and hyphens only; it must start with the type prefix and be unique.
- type: always "` + typ + `".
- topic: one lowercase word or short phrase, for example education, environment, technology, health, urban, work, culture, science, transport, media, society, housing, tourism, food, sport, finance, law, family.
- cefr: "B1", "B2" or "C1".
- tier: integer 1 to 4, where 1 is the most useful material for a learner targeting IELTS 6.5.
- source: always "generated".
- prompt: the instruction shown to the learner.

Content rules:
- No em dash (the character U+2014). Use commas, full stops or brackets.
- No emojis.
- British spelling in text content.
- Never use a real person's phone number, address or reference. Use reserved or fictional ranges.
- Do not include personal data or claims about real named people.
- Keep the English natural and at B2 or C1 level unless a simpler item is requested.`

	switch typ {
	case eval.TypeVocab:
		if fixedTargets {
			return common + `

This batch is VOCABULARY IN CONTEXT with a fixed target list. The user message lists the target words. Each object also needs:
- context: one natural sentence that contains the exact target word.
- target_word: the target exactly as written in the list.
- intended_meaning: a short, precise definition of the word as used in the context.
- prompt: "Explain what \"WORD\" means in this sentence, using your own words."

Write exactly one item for each target word. Do not add, skip, rename or inflect a target. Use the tier and CEFR shown for that word. Vary the topics and make the context clear enough to infer the meaning. Before returning an item, read the context once and check that it is grammatical and natural, and that the target word is used in the correct form and meaning.`
		}
		return common + `

This batch is VOCABULARY IN CONTEXT. Each object also needs:
- context: one natural sentence that contains the target word exactly.
- target_word: a single B2 or B2+ academic word (not a phrase).
- intended_meaning: a short, precise definition of the word as used in the context.
- prompt: "Explain what \"WORD\" means in this sentence, using your own words."

Choose useful academic words that an IELTS learner would meet in reading and writing. Do not reuse any target word in the avoid list. Vary the topics. The context must make the meaning clear enough to infer.`
	case eval.TypeWriting:
		return common + `

This batch is SHORT WRITING. Each object also needs:
- prompt: one original IELTS-style short writing task that asks for an opinion, a comparison, a problem and solution, or a short explanation. It must be answerable in 2 to 4 sentences.
- expected_length: for example "2 to 3 sentences" or "about 60 words".
- criteria: 2 or 3 short points the answer should cover.

Do not reuse any prompt in the avoid list. Vary the topics and task types. Do not ask for a full IELTS essay.`
	case eval.TypeListening:
		return common + `

This batch is LISTENING DICTATION. Each object also needs:
- listening_mode: "text" or "numeric".
- listening_kind: for text one of word, phrase, sentence. For numeric one of phone, address, price, date, time, postcode, reference, quantity, percentage, distance, weight, duration, room.
- audio_text: exactly what the speech synthesiser will say.
- answer_text: the canonical written answer. For text items it must equal audio_text.
- accepted_forms: an array of other correct renderings, mainly for numbers. Use [] for text items unless a number or abbreviation has a natural variant.
- prompt: "Type the word you hear.", "Type the phrase you hear." or "Type the sentence you hear." for text; for numeric use a specific instruction such as "Type the phone number you hear. Digits are fine."

Listening rules:
- Mix about 40 percent text and 60 percent numeric. When the batch has at least 6 items, cover at least 4 different numeric kinds.
- Text items should sound like academic or everyday IELTS listening: short announcements, instructions, campus situations, travel, accommodation, health, work and study.
- For numeric items, audio_text MUST contain no digits and no currency or percent symbols. Spell every digit and letter as words so the voice is controlled, and use commas for natural pauses. Example: "oh two oh, seven nine four six, oh one two three".
- answer_text is the formatted value, for example "020 7946 0123", "£12.50", "23 March 1998", "7:30 pm", "BS8 2QT", "AX7 492K", "three hundred and forty-two", "17.5 per cent".
- Use reserved or clearly fictional data. UK phones: 020 7946 0xxx or 07700 900xxx. Australia: 02 5550 0xxx or 04 5550 0xxx. US or Canada: area code plus 555-01xx. New Zealand: 09 555 0xxx. Ireland: 01 555 0xxx.
- Addresses must include a flat or unit, an invented house number, a street, a city and a plausible postcode.
- Vary the voices' likely difficulties: similar-sounding numbers, letters that are easy to confuse, and natural connected speech.
- Do not reuse any audio_text in the avoid list.`
	default:
		return common
	}
}

func userPrompt(typ string, count int, idx *store.ContentIndex, words []WordSpec) string {
	var b strings.Builder
	if len(words) > 0 {
		fmt.Fprintf(&b, "Generate exactly %d new vocab items, one for each target word below.\n", count)
		b.WriteString("Target words:\n")
		for _, w := range words {
			b.WriteString("- ")
			b.WriteString(w.Word)
			var hints []string
			if w.Tier > 0 {
				hints = append(hints, fmt.Sprintf("tier %d", w.Tier))
			}
			if w.CEFR != "" {
				hints = append(hints, w.CEFR)
			}
			if len(hints) > 0 {
				fmt.Fprintf(&b, " (%s)", strings.Join(hints, ", "))
			}
			b.WriteByte('\n')
		}
		b.WriteString("Every target must appear exactly once as target_word and in its context. Do not add other items.\n")
	} else {
		fmt.Fprintf(&b, "Generate exactly %d new %s items.\n", count, typ)
	}
	if len(idx.Keys) > 0 {
		keys := make([]string, 0, len(idx.Keys))
		for k := range idx.Keys {
			keys = append(keys, k)
		}
		// The prompt only needs a representative avoid list. Sending the whole
		// bank would waste tokens and is not needed because duplicates are also
		// filtered after generation.
		if len(keys) > 150 {
			keys = keys[:150]
		}
		fmt.Fprintf(&b, "Avoid these existing %s keys: %s\n", typ, strings.Join(keys, " | "))
	}
	b.WriteString("Return the JSON array now.")
	return b.String()
}
