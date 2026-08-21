package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestVersionMatchesChangelog keeps About / VERSION / CHANGELOG honest.
// The old hardcoded AppVersion="1.0.5" in version.go drifted across releases;
// VERSION is now the single source of truth and must match the latest
// "## [x.y.z]" section in CHANGELOG.md.
func TestVersionMatchesChangelog(t *testing.T) {
	verBytes, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	fileVer := strings.TrimSpace(string(verBytes))
	if fileVer == "" {
		t.Fatal("VERSION file is empty")
	}
	if got := strings.TrimSpace(AppVersion); got != fileVer {
		t.Fatalf("AppVersion=%q but VERSION file=%q (embed drift?)", got, fileVer)
	}

	cl, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	re := regexp.MustCompile(`(?m)^## \[([0-9]+\.[0-9]+\.[0-9]+)\]`)
	m := re.FindSubmatch(cl)
	if m == nil {
		t.Fatal("no ## [x.y.z] section found in CHANGELOG.md")
	}
	latest := string(m[1])
	if fileVer != latest {
		t.Fatalf("VERSION=%q but latest CHANGELOG section is [%s] — bump both together so About never lies", fileVer, latest)
	}
}
