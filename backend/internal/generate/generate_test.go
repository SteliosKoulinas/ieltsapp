package generate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ielts/backend/internal/eval"
	"ielts/backend/internal/store"
	"ielts/backend/internal/store/storetest"
)

type fakeLLM struct {
	out string
	err error
}

func (f fakeLLM) Generate(ctx context.Context, system, user string) (string, error) {
	return f.out, f.err
}

func seededStore(t *testing.T) *store.Store {
	t.Helper()
	st := storetest.New(t)
	ctx := context.Background()
	if _, err := st.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SeedListening(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestParseItems(t *testing.T) {
	items := `[{"a":1},{"b":2}]`
	cases := map[string]string{
		"plain":     items,
		"fence":     "```json\n" + items + "\n```",
		"wrapper":   `{"items":` + items + `}`,
		"surround":  "Here is the batch:\n" + items + "\nDone.",
		"whitespace": "\n\n " + items + " \n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseItems(raw)
			if err != nil || len(got) != 2 {
				t.Fatalf("parseItems: len=%d err=%v", len(got), err)
			}
		})
	}
	if _, err := parseItems("not json"); err == nil {
		t.Error("non-JSON output should be rejected")
	}
}

func TestRunVocabFiltersDuplicatesAndInvalidItems(t *testing.T) {
	st := seededStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Two items are valid, one repeats an existing word and an existing key,
	// one has a context without the target word, and one carries a slug with
	// the wrong prefix. Only the two valid ones may be kept.
	out := `[
	  {"slug":"vocab-bad-context","context":"This sentence does not contain it.","target_word":"juxtapose","intended_meaning":"place side by side","topic":"art","cefr":"C1","tier":1},
	  {"slug":"vocab-duplicate-word","context":"Governments allocate funds to schools.","target_word":"allocate","intended_meaning":"give out","topic":"education","cefr":"B2","tier":1},
	  {"slug":"vocab-juxtapose","context":"The exhibition juxtaposes old maps with satellite images.","target_word":"juxtapose","intended_meaning":"place two things side by side to compare them","topic":"culture","cefr":"C1","tier":1},
	  {"slug":"word-ubiquitous","context":"Smartphones are ubiquitous in modern life.","target_word":"ubiquitous","intended_meaning":"found everywhere","topic":"technology","cefr":"C1","tier":1},
	  {"slug":"vocab-juxtapose-again","context":"The essay juxtaposes two arguments.","target_word":"juxtapose","intended_meaning":"compare side by side","topic":"education","cefr":"C1","tier":1},
	  {"slug":"vocab-ubiquitous","context":"Smartphones are ubiquitous in modern life.","target_word":"ubiquitous","intended_meaning":"found everywhere","topic":"technology","cefr":"C1","tier":1}
	]`
	res, err := Run(ctx, fakeLLM{out: out}, st, Options{Type: eval.TypeVocab, Count: 2, OutDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 6 || res.Accepted != 2 || res.Skipped != 4 || res.Inserted != 2 {
		t.Errorf("result: %+v", res)
	}
	if res.Path == "" {
		t.Fatal("a batch file should have been written")
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("batch file missing: %v", err)
	}
	idx, err := st.ContentIndex(ctx, eval.TypeVocab)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"juxtapose", "ubiquitous"} {
		if !idx.Keys[key] {
			t.Errorf("new word %q should be in the content index", key)
		}
	}
}

func TestRunListeningKeepsTextAndNumeric(t *testing.T) {
	st := seededStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	// "allocate" already exists as a listening word, so the first item is a
	// duplicate by audio text even though its slug is new.
	out := `[
	  {"slug":"listen-word-allocate-copy","listening_mode":"text","listening_kind":"word","audio_text":"allocate","answer_text":"allocate","accepted_forms":[],"prompt":"Type the word you hear.","topic":"education","cefr":"B2","tier":1},
	  {"slug":"listen-room-418","listening_mode":"numeric","listening_kind":"room","audio_text":"room four hundred and eighteen","answer_text":"Room 418","accepted_forms":["418"],"prompt":"Type the room number you hear.","topic":"housing","cefr":"B1","tier":1},
	  {"slug":"listen-sentence-library-card","listening_mode":"text","listening_kind":"sentence","audio_text":"Your library card expires at the end of the month.","answer_text":"Your library card expires at the end of the month.","accepted_forms":[],"prompt":"Type the sentence you hear.","topic":"study","cefr":"B2","tier":1}
	]`
	res, err := Run(ctx, fakeLLM{out: out}, st, Options{Type: eval.TypeListening, Count: 2, OutDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 3 || res.Accepted != 2 || res.Skipped != 1 || res.Inserted != 2 {
		t.Errorf("result: %+v", res)
	}
	idx, err := st.ContentIndex(ctx, eval.TypeListening)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"room four hundred and eighteen", "your library card expires at the end of the month."} {
		if !idx.Keys[key] {
			t.Errorf("new audio %q should be in the content index", key)
		}
	}
}

func TestRunRejectsBadOptionsAndEmptyOutput(t *testing.T) {
	st := seededStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	for _, opts := range []Options{
		{Type: "grammar", Count: 1, OutDir: dir},
		{Type: eval.TypeVocab, Count: 0, OutDir: dir},
		{Type: eval.TypeVocab, Count: 31, OutDir: dir},
		{Type: eval.TypeVocab, Count: 1, OutDir: ""},
	} {
		if _, err := Run(ctx, fakeLLM{out: "[]"}, st, opts); err == nil {
			t.Errorf("options should be rejected: %+v", opts)
		}
	}

	res, err := Run(ctx, fakeLLM{out: `[{"slug":"vocab-only-duplicate","context":"Governments allocate funds.","target_word":"allocate","intended_meaning":"give","topic":"education","cefr":"B2","tier":1}]`},
		st, Options{Type: eval.TypeVocab, Count: 1, OutDir: dir})
	if err == nil || res == nil || res.Accepted != 0 {
		t.Fatalf("a batch of only duplicates should fail without writing: res=%+v err=%v", res, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("nothing should be written when no item is valid, found %d files", len(entries))
	}
	if !strings.Contains(err.Error(), "no new valid items") {
		t.Errorf("error should explain why nothing was written: %v", err)
	}
}

func TestBatchFileNameIsUnderTheOutputDirectory(t *testing.T) {
	st := seededStore(t)
	dir := t.TempDir()
	out := `[{"slug":"vocab-tenacious","context":"She was tenacious in her search for evidence.","target_word":"tenacious","intended_meaning":"persistent","topic":"society","cefr":"C1","tier":1}]`
	res, err := Run(context.Background(), fakeLLM{out: out}, st, Options{Type: eval.TypeVocab, Count: 1, OutDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(dir, res.Path)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Errorf("batch file should stay inside the output directory: %q", res.Path)
	}
}
