package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// testDataDirOverride allows tests to override the data directory.
// When set to a non-empty string, it will be used instead of the default path.
// This should only be set in test code, never in production.
var testDataDirOverride string

// dataDirLogFn is a package-level log hook (A5). When unset, messages go to the
// standard logger. The app wires this to its in-UI console at startup so every
// data-dir fallback / migration event is visible rather than silent.
var dataDirLogFn func(string)

// SetDataDirLogger installs the sink for data-dir events (fallbacks, migration).
func SetDataDirLogger(fn func(string)) { dataDirLogFn = fn }

func datadirLog(msg string) {
	if dataDirLogFn != nil {
		dataDirLogFn(msg)
		return
	}
	log.Printf("%s", msg)
}

// getHologramDataDir returns the base data directory for HOLOGRAM.
// Defaults to ~/.dero/hologram and falls back to the current working directory.
// If testDataDirOverride is set (for testing), it uses that instead.
//
// NOTE: this is the ONLY sanctioned os.Getwd() fallback in the codebase — the A14
// grep-gate excepts this file. Every other component must derive its paths from
// getHologramDataDir()/getDatashardsDir(), never from os.Getwd() directly.
func getHologramDataDir() string {
	// Allow tests to override the data directory
	if testDataDirOverride != "" {
		_ = os.MkdirAll(testDataDirOverride, 0755)
		return testDataDirOverride
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		base := filepath.Join(homeDir, ".dero", "hologram")
		if mkErr := os.MkdirAll(base, 0755); mkErr == nil {
			return base
		} else {
			datadirLog(fmt.Sprintf("[WARN] data-dir: cannot create %s (%v); falling back to working dir — HOLOGRAM state may be written outside ~/.dero/hologram", base, mkErr))
		}
	} else {
		datadirLog(fmt.Sprintf("[WARN] data-dir: os.UserHomeDir() failed (%v); falling back to working dir — HOLOGRAM state may be written outside ~/.dero/hologram", err))
	}

	wd, err := os.Getwd()
	if err == nil {
		datadirLog(fmt.Sprintf("[WARN] data-dir: using working dir %q as HOLOGRAM data dir", wd))
		return wd
	}

	datadirLog("[WARN] data-dir: falling back to \".\" as HOLOGRAM data dir")
	return "."
}

// getDatashardsDir returns the datashards directory, creating it if needed.
func getDatashardsDir() string {
	dir := filepath.Join(getHologramDataDir(), "datashards")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// expandTilde expands a leading ~ or ~/ to the user's home directory. Go does not
// expand ~ itself, so a config value like "~/.dero/mainnet" would otherwise create
// a directory literally named "~". Returns the input unchanged when it has no ~
// prefix or the home dir cannot be resolved.
func expandTilde(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// legacyMigrationOnce guards the one-time best-effort migration.
var legacyMigrationOnce sync.Once

// migrateLegacyDatashards copies legacy working-dir datashards state into the
// canonical datashards dir (A1/A12). It copies ONLY the named feature stores,
// copy-if-absent (never overwriting canonical data), and NEVER touches wallets/
// or settings/. Best-effort and idempotent; safe to call from multiple init paths.
func migrateLegacyDatashards() {
	legacyMigrationOnce.Do(func() {
		canonical := getDatashardsDir()

		wd, err := os.Getwd()
		if err != nil {
			return
		}
		legacy := filepath.Join(wd, "datashards")
		if legacy == canonical {
			return // already running from the canonical location
		}
		if _, err := os.Stat(legacy); err != nil {
			return // no legacy datashards to migrate
		}

		// Named entries only. wallets/ and settings/ are deliberately excluded.
		for _, name := range []string{"content_filter", "time_travel", "search_exclusions.json"} {
			src := filepath.Join(legacy, name)
			dst := filepath.Join(canonical, name)
			if _, err := os.Stat(src); err != nil {
				continue // no legacy copy of this entry
			}
			if _, err := os.Stat(dst); err == nil {
				continue // copy-if-absent: canonical already has it, never overwrite
			}
			if err := copyTree(src, dst); err != nil {
				datadirLog(fmt.Sprintf("[MIGRATE] skipped %s (%v)", name, err))
				continue
			}
			datadirLog(fmt.Sprintf("[MIGRATE] copied legacy %s -> %s", src, dst))
		}
	})
}

// copyTree recursively copies a file or directory, never following wallets/ or
// settings/ subtrees (defense-in-depth for the migration's exclusion guarantee).
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFileNoClobber(src, dst, info)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == "wallets" || e.Name() == "settings" {
			continue // never migrate wallet or settings data
		}
		if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFileNoClobber(src, dst string, info os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// O_EXCL: never clobber an existing canonical file.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
