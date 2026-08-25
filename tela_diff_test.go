package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// describeDiff renders a diff compactly so failures are readable.
func describeDiff(diff []map[string]interface{}) string {
	var b strings.Builder
	for _, d := range diff {
		switch d["type"] {
		case "modified":
			fmt.Fprintf(&b, "\n  [%d] modified -%q +%q", d["line"], d["oldContent"], d["newContent"])
		case "gap":
			fmt.Fprintf(&b, "\n  gap %v", d["count"])
		default:
			fmt.Fprintf(&b, "\n  [%d] %v %q", d["line"], d["type"], d["content"])
		}
	}
	if b.Len() == 0 {
		return " (no changes)"
	}
	return b.String()
}

// changedRows strips the context and gap framing so assertions about the
// CHANGES are not coupled to how much surrounding file the diff carries.
func changedRows(diff []map[string]interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, d := range diff {
		if d["type"] != "context" && d["type"] != "gap" {
			out = append(out, d)
		}
	}
	return out
}

func lineNo(t *testing.T, entry map[string]interface{}) int {
	t.Helper()
	n, ok := entry["line"].(int)
	if !ok {
		t.Fatalf("entry %v has no int \"line\" key", entry)
	}
	return n
}

func content(t *testing.T, entry map[string]interface{}) string {
	t.Helper()
	s, ok := entry["content"].(string)
	if !ok {
		t.Fatalf("entry %v has no string \"content\" key", entry)
	}
	return s
}

// TestGenerateDiff_SingleLineInsertIsSingleChange is the regression this whole
// change exists for. The old positional implementation compared oldLines[i]
// against newLines[i], so a one-line insert at the top shifted every following
// line and reported the entire file as rewritten.
func TestGenerateDiff_SingleLineInsertIsSingleChange(t *testing.T) {
	old := "line1\nline2\nline3\n"
	updated := "NEW\nline1\nline2\nline3\n"

	diff := changedRows(generateDiff(old, updated))

	if len(diff) != 1 {
		t.Fatalf("inserting one line at the top produced %d changes, want 1:%s", len(diff), describeDiff(diff))
	}
	if diff[0]["type"] != "added" {
		t.Errorf("type = %v, want \"added\"", diff[0]["type"])
	}
	if got := content(t, diff[0]); got != "NEW" {
		t.Errorf("content = %q, want %q", got, "NEW")
	}
	if got := lineNo(t, diff[0]); got != 1 {
		t.Errorf("line = %d, want 1", got)
	}
}

// TestGenerateDiff_TopInsertLargeFile is the same defect at the scale that made
// the diff viewer useless: the old code reported 501 changed lines here.
func TestGenerateDiff_TopInsertLargeFile(t *testing.T) {
	const n = 500

	lines := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	old := strings.Join(lines, "\n") + "\n"
	updated := "NEW\n" + old

	diff := changedRows(generateDiff(old, updated))

	if len(diff) != 1 {
		t.Fatalf("inserting one line at the top of a %d-line file produced %d changes, want 1", n, len(diff))
	}
	if diff[0]["type"] != "added" || content(t, diff[0]) != "NEW" || lineNo(t, diff[0]) != 1 {
		t.Errorf("unexpected entry: %v", diff[0])
	}
}

func TestGenerateDiff_TopDeleteIsSingleChange(t *testing.T) {
	diff := changedRows(generateDiff("z\na\nb\n", "a\nb\n"))

	if len(diff) != 1 {
		t.Fatalf("deleting the first line produced %d changes, want 1:%s", len(diff), describeDiff(diff))
	}
	if diff[0]["type"] != "removed" {
		t.Errorf("type = %v, want \"removed\"", diff[0]["type"])
	}
	if got := content(t, diff[0]); got != "z" {
		t.Errorf("content = %q, want %q", got, "z")
	}
	if got := lineNo(t, diff[0]); got != 1 {
		t.Errorf("line = %d, want 1", got)
	}
}

func TestGenerateDiff_MiddleInsertIsSingleChange(t *testing.T) {
	diff := changedRows(generateDiff("a\nb\nc\n", "a\nX\nb\nc\n"))

	if len(diff) != 1 {
		t.Fatalf("inserting one line mid-file produced %d changes, want 1:%s", len(diff), describeDiff(diff))
	}
	if diff[0]["type"] != "added" || content(t, diff[0]) != "X" {
		t.Errorf("unexpected entry: %v", diff[0])
	}
	if got := lineNo(t, diff[0]); got != 2 {
		t.Errorf("line = %d, want 2", got)
	}
}

// TestGenerateDiff_AppendWithoutTrailingNewline covers normalizeForDiff.
// go-diff hashes each line including its "\n", so without normalization "b" and
// "b\n" are different lines and appending "c" falsely reports "b" as changed.
func TestGenerateDiff_AppendWithoutTrailingNewline(t *testing.T) {
	diff := changedRows(generateDiff("a\nb", "a\nb\nc"))

	if len(diff) != 1 {
		t.Fatalf("appending one line to a file with no trailing newline produced %d changes, want 1:%s",
			len(diff), describeDiff(diff))
	}
	if diff[0]["type"] != "added" {
		t.Errorf("type = %v, want \"added\"", diff[0]["type"])
	}
	if got := content(t, diff[0]); got != "c" {
		t.Errorf("content = %q, want %q", got, "c")
	}
	if got := lineNo(t, diff[0]); got != 3 {
		t.Errorf("line = %d, want 3", got)
	}
}

// TestGenerateDiff_TrailingNewlineOnlyIsNoChange: adding just a terminating
// newline must not report the last line as modified.
func TestGenerateDiff_TrailingNewlineOnlyIsNoChange(t *testing.T) {
	diff := generateDiff("a\nb", "a\nb\n")

	if len(diff) != 0 {
		t.Errorf("adding only a trailing newline produced %d changes, want 0:%s", len(diff), describeDiff(diff))
	}
}

func TestGenerateDiff_BlankLineInsert(t *testing.T) {
	diff := changedRows(generateDiff("a\nb\n", "a\n\nb\n"))

	if len(diff) != 1 {
		t.Fatalf("inserting a blank line produced %d changes, want 1:%s", len(diff), describeDiff(diff))
	}
	if diff[0]["type"] != "added" {
		t.Errorf("type = %v, want \"added\"", diff[0]["type"])
	}
	if got := content(t, diff[0]); got != "" {
		t.Errorf("content = %q, want empty string", got)
	}
}

// TestGenerateDiff_MovedBlock: swapping two blocks correctly renders as one
// block relocated (inserted in its new place, removed from its old one) with
// the untouched block recognised as common. The old positional implementation
// instead paired A1 with B1, A2 with B2 ... producing six "modified" entries
// that each juxtaposed two unrelated lines. So the tell is the entry TYPES, not
// the count — both implementations touch six lines here.
func TestGenerateDiff_MovedBlock(t *testing.T) {
	old := "A1\nA2\nA3\nB1\nB2\nB3\n"
	updated := "B1\nB2\nB3\nA1\nA2\nA3\n"

	diff := generateDiff(old, updated)

	counts := map[string]int{}
	for _, d := range diff {
		counts[d["type"].(string)]++
	}

	if counts["modified"] != 0 {
		t.Errorf("a moved block reported %d modified lines, want 0 — unrelated lines were paired:%s",
			counts["modified"], describeDiff(diff))
	}
	if counts["added"] != 3 || counts["removed"] != 3 {
		t.Errorf("got %d added / %d removed, want 3 / 3:%s", counts["added"], counts["removed"], describeDiff(diff))
	}
}

// TestGenerateDiff_LineNumbersTrackBothSides checks that an insert followed by a
// modification numbers the modification against the NEW file, which is what the
// UI labels ("L{change.line}").
func TestGenerateDiff_LineNumbersTrackBothSides(t *testing.T) {
	diff := changedRows(generateDiff("a\nb\nc\n", "X\na\nB\nc\n"))

	if len(diff) != 2 {
		t.Fatalf("got %d changes, want 2:%s", len(diff), describeDiff(diff))
	}

	if diff[0]["type"] != "added" || lineNo(t, diff[0]) != 1 || content(t, diff[0]) != "X" {
		t.Errorf("first entry = %v, want added \"X\" at line 1", diff[0])
	}

	if diff[1]["type"] != "modified" {
		t.Fatalf("second entry type = %v, want \"modified\"", diff[1]["type"])
	}
	if lineNo(t, diff[1]) != 3 {
		t.Errorf("modified line = %d, want 3 (its position in the NEW file)", lineNo(t, diff[1]))
	}
	if diff[1]["oldContent"] != "b" || diff[1]["newContent"] != "B" {
		t.Errorf("modified entry = %v, want -\"b\" +\"B\"", diff[1])
	}
	if diff[1]["oldLine"] != 2 || diff[1]["newLine"] != 3 {
		t.Errorf("oldLine/newLine = %v/%v, want 2/3", diff[1]["oldLine"], diff[1]["newLine"])
	}
}

// TestGenerateDiff_WireShape pins the keys the Svelte consumer reads.
// VersionHistory.svelte reads type, line, and either content or
// oldContent+newContent. Dropping any of them breaks the UI silently — the
// frontend has no type checking.
func TestGenerateDiff_WireShape(t *testing.T) {
	diff := generateDiff("keep\ndrop\nchange me\n", "keep\nchanged\nnew tail\nextra\n")

	if len(diff) == 0 {
		t.Fatal("expected changes")
	}

	for i, entry := range diff {
		typ, ok := entry["type"].(string)
		if !ok {
			t.Fatalf("entry %d has no string \"type\"", i)
		}
		// A gap is a count of absent lines; it is the one row with no "line".
		if _, ok := entry["line"].(int); !ok && typ != "gap" {
			t.Errorf("entry %d (%s) has no int \"line\"", i, typ)
		}
		switch typ {
		case "added", "removed":
			if _, ok := entry["content"].(string); !ok {
				t.Errorf("entry %d (%s) has no string \"content\"", i, typ)
			}
		case "modified":
			if _, ok := entry["oldContent"].(string); !ok {
				t.Errorf("entry %d (modified) has no string \"oldContent\"", i)
			}
			if _, ok := entry["newContent"].(string); !ok {
				t.Errorf("entry %d (modified) has no string \"newContent\"", i)
			}
		case "context":
			if _, ok := entry["content"].(string); !ok {
				t.Errorf("entry %d (context) has no string \"content\"", i)
			}
			if _, ok := entry["oldLine"].(int); !ok {
				t.Errorf("entry %d (context) has no int \"oldLine\"", i)
			}
			if _, ok := entry["newLine"].(int); !ok {
				t.Errorf("entry %d (context) has no int \"newLine\"", i)
			}
		case "gap":
			if n, ok := entry["count"].(int); !ok || n <= 0 {
				t.Errorf("entry %d (gap) count = %v, want a positive int", i, entry["count"])
			}
		default:
			t.Errorf("entry %d has unknown type %q; the UI only renders added/removed/modified/context/gap", i, typ)
		}
	}
}

func TestGenerateDiff_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		oldContent string
		newContent string
		wantLen    int
		wantTypes  []string
	}{
		{name: "both empty", oldContent: "", newContent: "", wantLen: 0},
		{name: "identical single line", oldContent: "only\n", newContent: "only\n", wantLen: 0},
		{name: "identical no trailing newline", oldContent: "only", newContent: "only", wantLen: 0},
		{
			name: "empty to three lines", oldContent: "", newContent: "a\nb\nc\n",
			wantLen: 3, wantTypes: []string{"added", "added", "added"},
		},
		{
			name: "three lines to empty", oldContent: "a\nb\nc\n", newContent: "",
			wantLen: 3, wantTypes: []string{"removed", "removed", "removed"},
		},
		{
			name: "whitespace-only file", oldContent: "\n\n\n", newContent: "\n\n",
			wantLen: 1, wantTypes: []string{"removed"},
		},
		{
			name: "single line replaced", oldContent: "before", newContent: "after",
			wantLen: 1, wantTypes: []string{"modified"},
		},
		{
			name: "CRLF line endings untouched", oldContent: "a\r\nb\r\n", newContent: "a\r\nB\r\n",
			wantLen: 1, wantTypes: []string{"modified"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := changedRows(generateDiff(tt.oldContent, tt.newContent))
			if len(diff) != tt.wantLen {
				t.Fatalf("got %d changes, want %d:%s", len(diff), tt.wantLen, describeDiff(diff))
			}
			for i, want := range tt.wantTypes {
				if diff[i]["type"] != want {
					t.Errorf("diff[%d].type = %v, want %s", i, diff[i]["type"], want)
				}
			}
		})
	}
}

// TestGenerateDiff_VeryLargeFile guards against the diff blowing up or hanging
// on realistic worst-case input. Both inputs sit under maxDiffLines, so this is
// the real comparison rather than the refusal.
func TestGenerateDiff_VeryLargeFile(t *testing.T) {
	const n = 5000

	var oldB, newB strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&oldB, "the quick brown fox line %d\n", i)
		// Change every 250th line so the diff has real work to do.
		if i%250 == 0 {
			fmt.Fprintf(&newB, "the quick brown fox line %d (edited)\n", i)
		} else {
			fmt.Fprintf(&newB, "the quick brown fox line %d\n", i)
		}
	}

	diff := changedRows(generateDiff(oldB.String(), newB.String()))

	// 20 edited lines -> 20 modified entries. Assert it is bounded well below the
	// file size; the old implementation's failure mode was "everything changed".
	if len(diff) == 0 || len(diff) > 100 {
		t.Fatalf("editing 20 of %d lines produced %d changes, want ~20", n, len(diff))
	}
	for i, entry := range diff {
		if entry["type"] != "modified" {
			t.Errorf("entry %d type = %v, want \"modified\"", i, entry["type"])
		}
	}

	// Same-size unrelated files: must terminate and stay well-formed.
	var unrelated strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&unrelated, "completely different content %d\n", i*7)
	}
	if got := generateDiff(oldB.String(), unrelated.String()); len(got) == 0 {
		t.Error("two entirely different files produced no changes")
	}
}

// TestGenerateFileDiffs_DeterministicOrder: generateFileDiffs used to range over
// a map, so the same comparison listed its files in a different order on every
// call and the diff panel reshuffled on each render.
func TestGenerateFileDiffs_DeterministicOrder(t *testing.T) {
	filesA := map[string]string{
		"index.html": "<html>a</html>",
		"app.js":     "let a = 1;",
		"style.css":  "body{}",
		"gone.txt":   "removed",
		"util.js":    "export const x = 1;",
		"README.md":  "# old",
	}
	filesB := map[string]string{
		"index.html": "<html>b</html>",
		"app.js":     "let a = 2;",
		"style.css":  "body{color:red}",
		"added.svg":  "<svg/>",
		"util.js":    "export const x = 2;",
		"README.md":  "# new",
	}

	want := []string{"README.md", "added.svg", "app.js", "gone.txt", "index.html", "style.css", "util.js"}

	for i := 0; i < 25; i++ {
		diffs := generateFileDiffs(filesA, filesB)
		got := make([]string, 0, len(diffs))
		for _, d := range diffs {
			got = append(got, d.FileName)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("call %d returned files in order %v, want sorted %v", i, got, want)
		}
	}
}

// TestGenerateFileDiffs_UsesRealDiff: an added file at the top of another file's
// content must not read as a whole-file rewrite through the file-diff path either.
func TestGenerateFileDiffs_UsesRealDiff(t *testing.T) {
	filesA := map[string]string{"app.js": "const a = 1;\nconst b = 2;\nconst c = 3;\n"}
	filesB := map[string]string{"app.js": "'use strict';\nconst a = 1;\nconst b = 2;\nconst c = 3;\n"}

	diffs := generateFileDiffs(filesA, filesB)
	if len(diffs) != 1 {
		t.Fatalf("got %d file diffs, want 1", len(diffs))
	}
	if diffs[0].Status != "modified" {
		t.Errorf("status = %s, want modified", diffs[0].Status)
	}
	if changes := changedRows(diffs[0].LineDiffs); len(changes) != 1 {
		t.Fatalf("prepending one line produced %d line changes, want 1:%s",
			len(changes), describeDiff(diffs[0].LineDiffs))
	}
}

func TestNormalizeForDiff(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""}, // must NOT become "\n" — that would invent a blank line
		{"a", "a\n"},
		{"a\n", "a\n"},
		{"a\nb", "a\nb\n"},
		{"\n", "\n"},
	}
	for _, tt := range tests {
		if got := normalizeForDiff(tt.in); got != tt.want {
			t.Errorf("normalizeForDiff(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitDiffChunk(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a\n", []string{"a"}},
		{"a\nb\n", []string{"a", "b"}},
		{"a\n\n", []string{"a", ""}},
		{"a", []string{"a"}}, // no trailing newline: last line of an unterminated file
	}
	for _, tt := range tests {
		if got := splitDiffChunk(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitDiffChunk(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestGenerateDiff_PastLineCapIsRefusedNotDegraded pins the refusal that
// replaced go-diff's silent quality collapse.
//
// Two measured problems sit above this cap, and neither is visible from the
// output. With go-diff's stock 1s deadline, 40,000 lines with a third of them
// changed returned 39,998 entries — every line "modified", which is the bisect
// deadline fallback emitting a whole-file delete-then-insert; 80,000 returned
// 79,999. The entry count also moved with machine load, so the same two inputs
// did not give the same diff twice. Removing the deadline makes it exact, and
// then the cost is the thing that is unbounded: an exact diff of two fully
// disjoint texts measured 0.43s at 10k lines, 1.7s at 20k, 6.9s at 40k and
// 10.8s at 50k, all of it blocking the caller.
//
// So the cap is what has teeth here, and this test only proves the cap. The
// deadline removal cannot be given a failing test at capped sizes: probed at
// 15k-25k lines across five change densities, deadline-on and deadline-off
// returned identical output every time. It stays because output that depends
// on how busy the machine is cannot be reasoned about at all.
func TestGenerateDiff_PastLineCapIsRefusedNotDegraded(t *testing.T) {
	const n = maxDiffLines + 5000

	var oldB, newB strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&oldB, "the quick brown fox line %d\n", i)
		if i%3 == 0 {
			fmt.Fprintf(&newB, "the quick brown fox line %d (edited)\n", i)
		} else {
			fmt.Fprintf(&newB, "the quick brown fox line %d\n", i)
		}
	}

	diff := generateDiff(oldB.String(), newB.String())

	if len(diff) != 1 {
		t.Fatalf("got %d entries, want a single notice — a diff this size is refused, not degraded", len(diff))
	}
	if diff[0]["type"] != "notice" {
		t.Fatalf("entry type = %v, want \"notice\"", diff[0]["type"])
	}
	if content, _ := diff[0]["content"].(string); content == "" {
		t.Fatal("the notice carries no text; an empty result is indistinguishable from \"no changes\"")
	}
}

// TestGenerateDiff_AtLineCapIsStillCompared: the cap is a ceiling, not a shrug.
// Content at the limit must still get a real line-by-line comparison.
func TestGenerateDiff_AtLineCapIsStillCompared(t *testing.T) {
	var oldB, newB strings.Builder
	// countLines counts a terminating newline as ending the last line, so
	// maxDiffLines lines of text is exactly at the limit.
	for i := 0; i < maxDiffLines; i++ {
		fmt.Fprintf(&oldB, "line %d\n", i)
		if i == 7 {
			fmt.Fprintf(&newB, "line %d changed\n", i)
		} else {
			fmt.Fprintf(&newB, "line %d\n", i)
		}
	}

	diff := changedRows(generateDiff(oldB.String(), newB.String()))

	if len(diff) != 1 {
		t.Fatalf("got %d changed entries, want 1 modified line", len(diff))
	}
	if diff[0]["type"] != "modified" {
		t.Fatalf("entry type = %v, want \"modified\" — the cap must not swallow a legitimate diff", diff[0]["type"])
	}
}

// TestGenerateFileDiffs_TrailingNewlineOnlyIsStated: two files that differ only
// by a terminating newline used to render as a file header with a "modified"
// badge above the words "No line changes" — a changed file with nothing changed.
// normalizeForDiff makes them equal on purpose, so the file diff has to say
// which byte moved.
func TestGenerateFileDiffs_TrailingNewlineOnlyIsStated(t *testing.T) {
	diffs := generateFileDiffs(
		map[string]string{"app.js": "a\nb"},
		map[string]string{"app.js": "a\nb\n"},
	)

	if len(diffs) != 1 {
		t.Fatalf("got %d file diffs, want 1", len(diffs))
	}
	if diffs[0].Status != "modified" {
		t.Fatalf("Status = %q, want modified — the bytes did change", diffs[0].Status)
	}
	if len(diffs[0].LineDiffs) != 1 {
		t.Fatalf("got %d line entries, want a single notice explaining the empty diff", len(diffs[0].LineDiffs))
	}
	if diffs[0].LineDiffs[0]["type"] != "notice" {
		t.Fatalf("entry type = %v, want \"notice\"", diffs[0].LineDiffs[0]["type"])
	}
}

func TestCountLines(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 1},
		{"a\nb", 2},
		{"a\nb\n", 2},
	}
	for _, tc := range cases {
		if got := countLines(tc.in); got != tc.want {
			t.Errorf("countLines(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// typeSeq reduces a diff to its type sequence so hunk-framing assertions read
// as the shape they expect.
func typeSeq(diff []map[string]interface{}) []string {
	out := make([]string, 0, len(diff))
	for _, d := range diff {
		out = append(out, d["type"].(string))
	}
	return out
}

// numberedFile builds n lines "line 1".."line n", with edit applied to the
// 1-based lines named in edits.
func numberedFile(n int, edits map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if text, ok := edits[i]; ok {
			fmt.Fprintf(&b, "%s\n", text)
		} else {
			fmt.Fprintf(&b, "line %d\n", i)
		}
	}
	return b.String()
}

// TestGenerateDiff_ContextSurroundsChange: a lone mid-file change carries at
// most 3 unchanged lines on each side, with one gap row for everything elided
// toward each end of the file.
//
// Mutation proof: with the flushContext() calls removed from the change cases
// (the ring never flushes), no context rows are emitted and this test fails on
// the expected type sequence.
func TestGenerateDiff_ContextSurroundsChange(t *testing.T) {
	old := numberedFile(21, nil)
	updated := numberedFile(21, map[int]string{11: "line 11 edited"})

	diff := generateDiff(old, updated)

	want := []string{"gap", "context", "context", "context", "modified", "context", "context", "context", "gap"}
	if got := typeSeq(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("type sequence = %v, want %v:%s", got, want, describeDiff(diff))
	}

	// 10 equal lines precede the change; 3 are context, 7 are the gap. Same on
	// the other side.
	if diff[0]["count"] != 7 || diff[8]["count"] != 7 {
		t.Errorf("gap counts = %v/%v, want 7/7", diff[0]["count"], diff[8]["count"])
	}
	// Context is real file lines with real numbers on both sides.
	if diff[1]["content"] != "line 8" || diff[1]["oldLine"] != 8 || diff[1]["newLine"] != 8 {
		t.Errorf("first context row = %v, want line 8 at 8/8", diff[1])
	}
	if diff[7]["content"] != "line 14" || diff[7]["oldLine"] != 14 || diff[7]["newLine"] != 14 {
		t.Errorf("last context row = %v, want line 14 at 14/14", diff[7])
	}
}

// TestGenerateDiff_ContextAtFileEdges: a change on the first and last line has
// nothing before or after it — no leading or trailing context, no gap rows of
// zero.
func TestGenerateDiff_ContextAtFileEdges(t *testing.T) {
	old := numberedFile(3, nil)
	updated := numberedFile(3, map[int]string{1: "first edited", 3: "last edited"})

	diff := generateDiff(old, updated)

	// The single equal line between the two changes joins their context runs.
	want := []string{"modified", "context", "modified"}
	if got := typeSeq(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("type sequence = %v, want %v:%s", got, want, describeDiff(diff))
	}
}

// TestGenerateDiff_HunksSixApartMerge: 2*3 context lines is exactly the span
// two adjacent hunks can share, so changes 6 equal lines apart merge — every
// equal line between them is context and no gap row appears.
//
// Mutation proof: lowering the merge threshold by one (join only at <= 5
// equal lines) splits these hunks and inserts a gap row, failing the expected
// type sequence here.
func TestGenerateDiff_HunksSixApartMerge(t *testing.T) {
	old := numberedFile(8, nil)
	updated := numberedFile(8, map[int]string{1: "first edited", 8: "last edited"})

	diff := generateDiff(old, updated)

	want := []string{"modified", "context", "context", "context", "context", "context", "context", "modified"}
	if got := typeSeq(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes 6 apart must merge into one hunk; type sequence = %v, want %v:%s",
			got, want, describeDiff(diff))
	}
}

// TestGenerateDiff_HunksFourApartMerge: comfortably inside the threshold, all
// 4 equal lines between the changes ride along as context.
func TestGenerateDiff_HunksFourApartMerge(t *testing.T) {
	old := numberedFile(6, nil)
	updated := numberedFile(6, map[int]string{1: "first edited", 6: "last edited"})

	diff := generateDiff(old, updated)

	want := []string{"modified", "context", "context", "context", "context", "modified"}
	if got := typeSeq(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes 4 apart must merge into one hunk; type sequence = %v, want %v:%s",
			got, want, describeDiff(diff))
	}
}

// TestGenerateDiff_HunksTenApartGap: past the threshold the hunks split — 3
// trailing context, a gap carrying exactly the elided count, 3 leading context.
//
// Mutation proof: raising the merge threshold by one (join at <= 7 equal
// lines) still splits these hunks but elides only 3 lines, failing the gap
// count assertion here.
func TestGenerateDiff_HunksTenApartGap(t *testing.T) {
	old := numberedFile(12, nil)
	updated := numberedFile(12, map[int]string{1: "first edited", 12: "last edited"})

	diff := generateDiff(old, updated)

	want := []string{"modified", "context", "context", "context", "gap", "context", "context", "context", "modified"}
	if got := typeSeq(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes 10 apart must split; type sequence = %v, want %v:%s", got, want, describeDiff(diff))
	}
	if diff[4]["count"] != 4 {
		t.Errorf("gap count = %v, want 4 — 10 equal lines minus 3 context on each side", diff[4]["count"])
	}
	// The context runs sit flush against their hunks: lines 2-4 close the first,
	// lines 9-11 open the second.
	if diff[1]["newLine"] != 2 || diff[3]["newLine"] != 4 || diff[5]["newLine"] != 9 || diff[7]["newLine"] != 11 {
		t.Errorf("context line numbers wrong:%s", describeDiff(diff))
	}
}

// TestGenerateDiff_GapsAccountForEveryLine: framing must conserve the file —
// changed rows plus context rows plus gap counts walk every line exactly once.
func TestGenerateDiff_GapsAccountForEveryLine(t *testing.T) {
	const n = 200
	old := numberedFile(n, nil)
	updated := numberedFile(n, map[int]string{40: "edited", 120: "also edited"})

	diff := generateDiff(old, updated)

	covered := 0
	for _, d := range diff {
		switch d["type"] {
		case "gap":
			covered += d["count"].(int)
		default:
			covered++
		}
	}
	if covered != n {
		t.Errorf("rows and gap counts cover %d lines, want %d:%s", covered, n, describeDiff(diff))
	}
}

// TestGenerateDiff_ShortTrailingContextNoGap: fewer than 3 equal lines after
// the last hunk all become context and no trailing gap row appears.
//
// Mutation proof: deleting the `else { diff = append(diff, pending...) }`
// branch in generateDiff's tail block silently drops the trailing context and
// fails the expected type sequence here.
func TestGenerateDiff_ShortTrailingContextNoGap(t *testing.T) {
	old := numberedFile(13, nil)
	updated := numberedFile(13, map[int]string{11: "line 11 edited"})

	diff := generateDiff(old, updated)

	want := []string{"gap", "context", "context", "context", "modified", "context", "context"}
	if got := typeSeq(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("type sequence = %v, want %v:%s", got, want, describeDiff(diff))
	}
	if diff[5]["content"] != "line 12" || diff[6]["content"] != "line 13" {
		t.Errorf("trailing context = %v / %v, want lines 12 and 13", diff[5], diff[6])
	}
}

// TestGenerateDiff_ContextLineNumbersSkewAfterInsert: an unpaired insert moves
// the two sides apart, so context rows after it must carry oldLine one behind
// newLine — a symmetric-hunk suite cannot see oldLine and newLine swapped.
//
// Mutation proof: emitting context rows with oldLine and newLine exchanged
// leaves every symmetric test green and fails only here.
func TestGenerateDiff_ContextLineNumbersSkewAfterInsert(t *testing.T) {
	old := numberedFile(5, nil)
	updated := "line 1\nline 2\ninserted line\nline 3\nline 4\nline 5\n"

	diff := generateDiff(old, updated)

	want := []string{"context", "context", "added", "context", "context", "context"}
	if got := typeSeq(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("type sequence = %v, want %v:%s", got, want, describeDiff(diff))
	}
	// Before the insert the sides agree; after it old lags new by one.
	for i, wantOld := range map[int]int{0: 1, 1: 2} {
		if diff[i]["oldLine"] != wantOld || diff[i]["newLine"] != wantOld {
			t.Errorf("leading context row %d = %v, want %d/%d", i, diff[i], wantOld, wantOld)
		}
	}
	for i, wantOld := range map[int]int{3: 3, 4: 4, 5: 5} {
		if diff[i]["oldLine"] != wantOld || diff[i]["newLine"] != wantOld+1 {
			t.Errorf("trailing context row %d = %v, want oldLine %d / newLine %d", i, diff[i], wantOld, wantOld+1)
		}
	}
}
