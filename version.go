package main

// Version information - set at build time via ldflags
// Example: go build -ldflags "-X main.AppVersion=1.0.0 -X main.BuildDate=2024-12-16 -X main.GitCommit=abc1234"
var (
	AppVersion = "1.0.0"
	BuildDate  = "dev"
	GitCommit  = "unknown"
)

// GetAppInfo returns version and build information for the About page
func (a *App) GetAppInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":      "Hologram",
		"version":   AppVersion,
		"buildDate": BuildDate,
		"gitCommit": GitCommit,
		"author":    "DHEBP",
		"website":   "https://github.com/DHEBP/HOLOGRAM",
		"description": "A native browser for the DERO decentralized web (TELA)",
	}
}
