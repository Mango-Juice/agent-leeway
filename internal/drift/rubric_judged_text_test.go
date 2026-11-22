package drift

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

// The judge sees only the machine block and the settings rows (BuildSystem), never Parts A prose or C. Edits to
// the human parts change the whole-file version id but must not change the judged text that was validated:
// rubric v2.1-b636272b (bundled; docs/23 P3 run with the Go tool) and v2.1-3fbcc065 (the same text in
// public/docs/rubric.md with export placeholders; docs/18 run). Since the Part C update the bundled id is
// recorded in validatedRubricIDs. The validated text is built with the frozen TaskV21.
const validatedJudgedTextSHA256 = "053d3bed9ad4f3f7d1298859456c1bd212e980b91c9cf07e63f83db97cf00abe"

// currentJudgedTextSHA256 pins the judged text in use: TaskRevision t2 (summaries in the user's language, owner
// decision 2026-11-22). It differs from the validated text only as TestTaskDiffersOnlyInSummaryLanguage allows.
const currentJudgedTextSHA256 = "2db4aa7ada4665b18f5c092adeb6343d73f4af1e6da7f3db9f043325865a5a44"

var validatedRubricIDs = []string{"v2.1-3fbcc065", "v2.1-b636272b"}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestBundledRubricJudgedTextIsTheValidatedOne(t *testing.T) {
	rb, err := LoadRubric("")
	if err != nil {
		t.Fatal(err)
	}
	if got := sha(buildSystem(TaskV21, rb.Body, rb.Settings)); got != validatedJudgedTextSHA256 {
		t.Fatalf("the validated judged text of the bundled rubric changed (sha256 %s); it was validated as %s (%v). Re-validate before shipping a changed machine block or settings table.", got, validatedJudgedTextSHA256, validatedRubricIDs)
	}
	if got := sha(BuildSystem(rb.Body, rb.Settings)); got != currentJudgedTextSHA256 {
		t.Fatalf("the judged text in use changed (sha256 %s, pinned %s); give it a new TaskRevision and re-check before shipping.", got, currentJudgedTextSHA256)
	}
	if !strings.HasSuffix(rb.Version, "+"+TaskRevision) {
		t.Errorf("version %s does not name the task revision", rb.Version)
	}
}

func TestParityUsesTheFrozenText(t *testing.T) {
	legacy(t)
	rb, err := LoadRubric("")
	if err != nil {
		t.Fatal(err)
	}
	if sha(BuildSystem(rb.Body, rb.Settings)) != validatedJudgedTextSHA256 || strings.Contains(rb.Version, "+") {
		t.Fatalf("parity mode must judge the validated text with the plain version id (%s)", rb.Version)
	}
}

// The new judged text differs from the validated one only in the language of the free-text summaries: the four
// "one sentence in English" placeholders and one added line after fix_sentence. Any other change fails.
func TestTaskDiffersOnlyInSummaryLanguage(t *testing.T) {
	rb, err := LoadRubric("")
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Split(buildSystem(TaskV21, rb.Body, rb.Settings), "\n")
	cur := strings.Split(BuildSystem(rb.Body, rb.Settings), "\n")
	added := "summary, verdict_reason: one sentence each, written in the language the user mainly writes in within the session " +
		"(the same rule as fix_sentence). Field names and enumerated values stay in English; quotes stay verbatim."
	if len(cur) != len(old)+1 || cur[len(cur)-1] != added {
		t.Fatalf("expected exactly one added last line, got %d -> %d lines, last %q", len(old), len(cur), cur[len(cur)-1])
	}
	changed := 0
	for i := range old {
		if old[i] == cur[i] {
			continue
		}
		changed++
		if strings.ReplaceAll(old[i], `"one sentence in English"`, `"one sentence in the user's language"`) != cur[i] {
			t.Errorf("line %d changed beyond the summary language:\n- %s\n+ %s", i+1, old[i], cur[i])
		}
	}
	if changed != 4 || strings.Count(strings.Join(cur, "\n"), "one sentence in English") != 0 {
		t.Errorf("%d lines changed, want 4", changed)
	}
}

// validatedRubricPath is the rubric file exactly as validated (v2.1-b636272b). Parity tests that compare with the
// Python reference's outputs, which carry that version id, load it instead of the bundled copy.
func validatedRubricPath() string {
	p, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "rubric-v2.1-b636272b.md"))
	return p
}

func TestValidatedFixtureAndBundledRubricJudgeTheSameText(t *testing.T) {
	val, err := LoadRubric(validatedRubricPath())
	if err != nil {
		t.Fatal(err)
	}
	if val.Version != "v2.1-b636272b+"+TaskRevision {
		t.Fatalf("fixture version %s, want v2.1-b636272b", val.Version)
	}
	cur, err := LoadRubric("")
	if err != nil {
		t.Fatal(err)
	}
	if BuildSystem(val.Body, val.Settings) != BuildSystem(cur.Body, cur.Settings) {
		t.Error("the bundled rubric's judged text differs from the validated rubric v2.1-b636272b")
	}
}

// public/docs/rubric.md (the export form, with the product-name placeholders unfilled; docs/18 measured it as
// v2.1-3fbcc065) differs from the bundled copy only outside the judged text, so it judges the same text.
// The file exists only in the private repository; the exported tree carries the substituted copy.
func TestPublicRubricJudgesTheSameText(t *testing.T) {
	p, _ := filepath.Abs(filepath.Join("..", "..", "public", "docs", "rubric.md"))
	pub, err := LoadRubric(p)
	if err != nil {
		t.Skip("no public/docs/rubric.md in this tree")
	}
	cur, _ := LoadRubric("")
	if BuildSystem(pub.Body, pub.Settings) != BuildSystem(cur.Body, cur.Settings) {
		t.Error("public/docs/rubric.md and the bundled rubric give different judged text")
	}
}
