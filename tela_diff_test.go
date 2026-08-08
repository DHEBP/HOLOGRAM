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
		default:
			fmt.Fprintf(&b, "\n  [%d] %v %q", d["line"], d["type"], d["content"])
		}
	}
	if b.Len() == 0 {
		return " (no changes)"
	}
	return b.String()
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

	diff := generateDiff(old, updated)

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

	diff := generateDiff(old, updated)

	if len(diff) != 1 {
		t.Fatalf("inserting one line at the top of a %d-line file produced %d changes, want 1", n, len(diff))
	}
	if diff[0]["type"] != "added" || content(t, diff[0]) != "NEW" || lineNo(t, diff[0]) != 1 {
		t.Errorf("unexpected entry: %v", diff[0])
	}
}

func TestGenerateDiff_TopDeleteIsSingleChange(t *testing.T) {
	diff := generateDiff("z\na\nb\n", "a\nb\n")

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
	diff := generateDiff("a\nb\nc\n", "a\nX\nb\nc\n")

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
	diff := generateDiff("a\nb", "a\nb\nc")

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
	diff := generateDiff("a\nb\n", "a\n\nb\n")

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
	diff := generateDiff("a\nb\nc\n", "X\na\nB\nc\n")

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
		if _, ok := entry["line"].(int); !ok {
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
		default:
			t.Errorf("entry %d has unknown type %q; the UI only renders added/removed/modified", i, typ)
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
			diff := generateDiff(tt.oldContent, tt.newContent)
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
// on realistic worst-case input. go-diff carries a 1s internal deadline; a
// timeout degrades quality but must still return a well-formed diff.
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

	diff := generateDiff(oldB.String(), newB.String())

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
	if len(diffs[0].LineDiffs) != 1 {
		t.Fatalf("prepending one line produced %d line changes, want 1:%s",
			len(diffs[0].LineDiffs), describeDiff(diffs[0].LineDiffs))
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
