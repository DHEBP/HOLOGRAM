package main

// Regression tests for the data-dir consolidation (redteam ledger
// redteam-hologram-datadir-consolidation.md). These lock the behaviour-observing
// checks the ledger mandated: tilde expansion, server-side node dir normalization
// (never leaking a literal "~"), the migration never touching wallets/settings and
// never clobbering, and the RemoveFile containment guard.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWriteDatashardFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cases := map[string]string{
		"~":          home,
		"~/foo":      filepath.Join(home, "foo"),
		"~/a/b":      filepath.Join(home, "a", "b"),
		"/abs/path":  "/abs/path",
		"relative/x": "relative/x",
		"":           "",
		"~notme":     "~notme", // only exact ~ or ~/ prefix expands
	}
	for in, want := range cases {
		if got := expandTilde(in); got != want {
			t.Errorf("expandTilde(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveNodeDataDirNormalizesAndNeverLeaksTilde(t *testing.T) {
	tmp := t.TempDir()
	testDataDirOverride = tmp
	defer func() { testDataDirOverride = "" }()

	if got := resolveNodeDataDir(""); got != tmp {
		t.Errorf(`resolveNodeDataDir("") = %q, want %q`, got, tmp)
	}
	if got := resolveNodeDataDir("   "); got != tmp {
		t.Errorf("resolveNodeDataDir(spaces) = %q, want %q", got, tmp)
	}
	if got, want := resolveNodeDataDir("sub"), filepath.Join(tmp, "sub"); got != want {
		t.Errorf(`resolveNodeDataDir("sub") = %q, want %q`, got, want)
	}
	if got := resolveNodeDataDir("/custom/dir"); got != "/custom/dir" {
		t.Errorf("resolveNodeDataDir(abs) = %q, want /custom/dir", got)
	}
	// No input should ever yield a path that would create a directory literally
	// named "~" (the O9 FirstRunWizard hazard).
	for _, in := range []string{"", "sub", "~/x", "~"} {
		got := resolveNodeDataDir(in)
		if got == "~" || strings.HasPrefix(got, "~/") {
			t.Errorf("resolveNodeDataDir(%q) = %q leaks a literal ~", in, got)
		}
	}
}

func TestMigrationCopyTreeSkipsWalletsAndSettings(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "canonical")

	mustWriteDatashardFile(t, filepath.Join(src, "content_filter", "0", "0.dfs"), "rules")
	mustWriteDatashardFile(t, filepath.Join(src, "wallets", "demo.db"), "SECRET-WALLET")
	mustWriteDatashardFile(t, filepath.Join(src, "settings", "settings.json"), "{}")

	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "content_filter", "0", "0.dfs")); err != nil {
		t.Errorf("content_filter should be migrated, but: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "wallets")); !os.IsNotExist(err) {
		t.Errorf("wallets/ must NOT be migrated (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "settings")); !os.IsNotExist(err) {
		t.Errorf("settings/ must NOT be migrated (err=%v)", err)
	}
}

func TestCopyFileNoClobberRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	mustWriteDatashardFile(t, src, "new")
	mustWriteDatashardFile(t, dst, "EXISTING")

	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := copyFileNoClobber(src, dst, info); err == nil {
		t.Fatal("copyFileNoClobber overwrote an existing file; migration must be copy-if-absent")
	}
	if got, _ := os.ReadFile(dst); string(got) != "EXISTING" {
		t.Errorf("dst was clobbered: got %q, want EXISTING", got)
	}
}

func TestRemovableUnderDatashardsGuard(t *testing.T) {
	tmp := t.TempDir()
	testDataDirOverride = tmp
	defer func() { testDataDirOverride = "" }()
	ds := getDatashardsDir()

	cases := []struct {
		path    string
		wantOK  bool
		wantWhy string
	}{
		{filepath.Join(ds, "clone", "app.html"), true, ""},
		{filepath.Join(ds, "tela", "x"), true, ""},
		{filepath.Join(ds, "wallets", "demo.db"), false, "protected"},
		{filepath.Join(ds, "settings", "settings.json"), false, "protected"},
		{ds, false, "outside"},                                        // the datashards root itself
		{filepath.Join(tmp, "elsewhere", "x"), false, "outside"},      // outside datashards
		{"/etc/passwd", false, "outside"},                             // absolute, far outside
		{filepath.Join(tmp, "datashardsX", "x"), false, "outside"},    // sibling prefix must NOT match
	}
	for _, c := range cases {
		ok, why := removableUnderDatashards(c.path)
		if ok != c.wantOK || (!ok && why != c.wantWhy) {
			t.Errorf("removableUnderDatashards(%q) = (%v,%q), want (%v,%q)", c.path, ok, why, c.wantOK, c.wantWhy)
		}
	}
}
