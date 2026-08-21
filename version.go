package main

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embeddedVersion string

// Version information.
//
// AppVersion comes from the VERSION file (single source of truth with CHANGELOG).
// Release CI may still override metadata via ldflags:
//
//	go build -ldflags "-X main.AppVersion=1.0.8 -X main.BuildDate=… -X main.GitCommit=…"
//
// Do not hardcode a release number here — bump VERSION (+ CHANGELOG) together.
// TestVersionMatchesChangelog fails CI if they drift.
var (
	AppVersion = strings.TrimSpace(embeddedVersion)
	BuildDate  = "dev"
	GitCommit  = "unknown"
)

// ReleaseDate is the official v1.0.0 public release — HOLOGRAM's birthday.
const ReleaseDate = "2026-04-18"

// GetAppInfo returns version and build information for the About page
func (a *App) GetAppInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":        "Hologram",
		"version":     AppVersion,
		"buildDate":   BuildDate,
		"gitCommit":   GitCommit,
		"releaseDate": ReleaseDate,
		"author":      "DHEBP",
		"website":     "https://github.com/DHEBP/HOLOGRAM",
		"description": "A native browser for the DERO decentralized web (TELA)",
	}
}
