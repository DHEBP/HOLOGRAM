package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DerodDownloader handles downloading and managing derod binaries
type DerodDownloader struct {
	app        *App
	baseDir    string // ~/.dero/hologram/derod/
	httpClient *http.Client
}

// GitHubRelease represents a GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// DownloadProgress represents download progress info
type DownloadProgress struct {
	TotalBytes      int64  `json:"totalBytes"`
	DownloadedBytes int64  `json:"downloadedBytes"`
	Percentage      int    `json:"percentage"`
	Status          string `json:"status"`
}

// ChecksumInfo represents parsed checksum data
type ChecksumInfo struct {
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
}

// NewDerodDownloader creates a new downloader instance
func NewDerodDownloader(app *App) *DerodDownloader {
	homeDir, _ := os.UserHomeDir()
	return &DerodDownloader{
		app:        app,
		baseDir:    filepath.Join(homeDir, ".dero", "hologram", "derod"),
		httpClient: &http.Client{},
	}
}

// GetBaseDir returns the base directory for derod storage
func (d *DerodDownloader) GetBaseDir() string {
	return d.baseDir
}

// GetLatestDeroRelease fetches the latest DERO release info from GitHub
func (d *DerodDownloader) GetLatestDeroRelease() (*GitHubRelease, error) {
	d.app.logToConsole("[NET] Fetching latest DERO release from GitHub...")

	resp, err := d.httpClient.Get("https://api.github.com/repos/deroproject/derohe/releases/latest")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	d.app.logToConsole(fmt.Sprintf("[PKG] Latest release: %s", release.TagName))
	return &release, nil
}

// GetPlatformAssetName returns the expected asset name for the current platform
func (d *DerodDownloader) GetPlatformAssetName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map to DERO's naming convention
	var osName, archName, ext string

	switch goos {
	case "darwin":
		// macOS uses universal binary (works on both Intel and Apple Silicon)
		osName = "darwin"
		archName = "universal"
	case "linux":
		osName = "linux"
		switch goarch {
		case "amd64":
			archName = "amd64"
		case "arm64":
			archName = "arm64"
		case "arm":
			archName = "arm7"
		default:
			archName = goarch
		}
	case "windows":
		osName = "windows"
		archName = "amd64"
	case "freebsd":
		osName = "freebsd"
		archName = "amd64"
	default:
		osName = goos
		archName = goarch
	}

	if goos == "windows" {
		ext = ".zip"
	} else {
		ext = ".tar.gz"
	}

	// DERO release assets are typically named like: dero_linux_amd64.tar.gz
	return fmt.Sprintf("dero_%s_%s%s", osName, archName, ext)
}

// GetChecksumFile downloads and parses the checksum.txt.signed file from a release
func (d *DerodDownloader) GetChecksumFile(release *GitHubRelease) (map[string]string, error) {
	checksums := make(map[string]string)

	// Find checksum.txt.signed asset
	var checksumURL string
	for _, asset := range release.Assets {
		if asset.Name == "checksum.txt.signed" {
			checksumURL = asset.BrowserDownloadURL
			break
		}
	}

	if checksumURL == "" {
		return nil, fmt.Errorf("checksum.txt.signed not found in release assets")
	}

	d.app.logToConsole("[VERIFY] Downloading checksum file...")

	resp, err := d.httpClient.Get(checksumURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download checksum file: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read checksum file: %w", err)
	}

	// Parse DERO signed message format
	// The checksums are base64 encoded between the header and signature
	content := string(body)

	// Find the base64 encoded section (after S: line, before END marker)
	lines := strings.Split(content, "\n")
	var base64Content string
	inPayload := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip header lines
		if strings.HasPrefix(line, "-----BEGIN") || strings.HasPrefix(line, "-----END") {
			continue
		}
		if strings.HasPrefix(line, "Address:") || strings.HasPrefix(line, "C:") || strings.HasPrefix(line, "S:") {
			if strings.HasPrefix(line, "S:") {
				inPayload = true
			}
			continue
		}
		// After S: line, collect base64 content
		if inPayload {
			base64Content += line
		}
	}

	if base64Content == "" {
		return nil, fmt.Errorf("no checksum data found in signed message")
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode checksum data: %w", err)
	}

	// Parse checksum lines: "SHA256 (filename) = hash"
	checksumLines := strings.Split(string(decoded), "\n")
	for _, line := range checksumLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse format: "SHA256 (filename) = hash"
		if strings.HasPrefix(line, "SHA256 (") {
			// Extract filename
			start := strings.Index(line, "(")
			end := strings.Index(line, ")")
			if start == -1 || end == -1 || end <= start {
				continue
			}
			filename := line[start+1 : end]

			// Extract hash (after " = ")
			hashStart := strings.Index(line, " = ")
			if hashStart == -1 {
				continue
			}
			hash := strings.TrimSpace(line[hashStart+3:])

			checksums[filename] = strings.ToLower(hash)
			d.app.logToConsole(fmt.Sprintf("[VERIFY] Found checksum for %s", filename))
		}
	}

	if len(checksums) == 0 {
		return nil, fmt.Errorf("no valid checksums parsed from checksum file")
	}

	return checksums, nil
}

// VerifyFileChecksum calculates SHA256 of a file and compares with expected hash
func (d *DerodDownloader) VerifyFileChecksum(filePath string, expectedHash string) error {
	d.app.logToConsole("[VERIFY] Calculating SHA256 checksum...")

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for verification: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))
	expectedHash = strings.ToLower(strings.TrimSpace(expectedHash))

	if calculatedHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, calculatedHash)
	}

	d.app.logToConsole("[OK] Checksum verified successfully")
	return nil
}

// FindAssetForPlatform finds the download URL for the current platform
func (d *DerodDownloader) FindAssetForPlatform(release *GitHubRelease) (string, int64, error) {
	expectedName := d.GetPlatformAssetName()
	d.app.logToConsole(fmt.Sprintf("[...] Looking for asset: %s", expectedName))

	for _, asset := range release.Assets {
		// Check for exact match or partial match
		assetLower := strings.ToLower(asset.Name)
		expectedLower := strings.ToLower(expectedName)

		if assetLower == expectedLower || strings.Contains(assetLower, strings.TrimSuffix(expectedLower, filepath.Ext(expectedLower))) {
			d.app.logToConsole(fmt.Sprintf("[OK] Found matching asset: %s (%d MB)", asset.Name, asset.Size/1024/1024))
			return asset.BrowserDownloadURL, asset.Size, nil
		}
	}

	// List available assets for debugging
	available := []string{}
	for _, asset := range release.Assets {
		available = append(available, asset.Name)
	}
	return "", 0, fmt.Errorf("no matching asset found for %s/%s. Available: %v", runtime.GOOS, runtime.GOARCH, available)
}

// DownloadDerod downloads derod from the given URL with progress updates and checksum verification
func (d *DerodDownloader) DownloadDerod(url string, version string, expectedChecksum string) error {
	d.app.logToConsole(fmt.Sprintf("⬇️ Downloading derod %s...", version))

	// Create version directory
	versionDir := filepath.Join(d.baseDir, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Determine archive type
	isZip := strings.HasSuffix(url, ".zip")

	// Download to temp file
	archivePath := filepath.Join(versionDir, "archive")
	if isZip {
		archivePath += ".zip"
	} else {
		archivePath += ".tar.gz"
	}

	// Perform download
	resp, err := d.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer resp.Body.Close()

	totalSize := resp.ContentLength
	d.app.logToConsole(fmt.Sprintf("[STATS] Total size: %d MB", totalSize/1024/1024))

	// Create output file
	out, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}

	// Download with progress tracking
	downloaded := int64(0)
	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				out.Close()
				return fmt.Errorf("failed to write: %w", writeErr)
			}
			downloaded += int64(n)

			// Log progress every ~10%
			if totalSize > 0 {
				pct := int(float64(downloaded) / float64(totalSize) * 100)
				if pct%10 == 0 {
					d.app.logToConsole(fmt.Sprintf("[Download] Progress: %d%% (%d/%d MB)", pct, downloaded/1024/1024, totalSize/1024/1024))
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			out.Close()
			return fmt.Errorf("download error: %w", err)
		}
	}
	out.Close()

	d.app.logToConsole("[OK] Download complete")

	// Verify checksum (mandatory for auto-download)
	if expectedChecksum != "" {
		if err := d.VerifyFileChecksum(archivePath, expectedChecksum); err != nil {
			// Clean up the potentially compromised file
			os.Remove(archivePath)
			return fmt.Errorf("SECURITY: %w - download aborted", err)
		}
	} else {
		d.app.logToConsole("[WARN] No checksum provided - skipping verification")
	}

	d.app.logToConsole("[...] Extracting archive...")

	// Extract archive
	if isZip {
		err = d.extractZip(archivePath, versionDir)
	} else {
		err = d.extractTarGz(archivePath, versionDir)
	}
	if err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	// Clean up archive
	os.Remove(archivePath)

	// Make binary executable
	derodPath := d.findDerodBinary(versionDir)
	if derodPath == "" {
		return fmt.Errorf("derod binary not found in extracted files")
	}

	if err := os.Chmod(derodPath, 0755); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	d.app.logToConsole(fmt.Sprintf("[OK] derod %s installed at: %s", version, derodPath))
	return nil
}

// extractTarGz extracts a tar.gz archive
func (d *DerodDownloader) extractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(target)
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// extractZip extracts a zip archive
func (d *DerodDownloader) extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// findDerodBinary searches for the derod binary in the extracted directory
func (d *DerodDownloader) findDerodBinary(dir string) string {
	// DERO binaries have platform-specific names:
	// - Linux: derod-linux-amd64, derod-linux-arm64
	// - macOS: derod-darwin
	// - Windows: derod-windows-amd64.exe
	var possibleNames []string

	switch runtime.GOOS {
	case "darwin":
		possibleNames = []string{"derod-darwin", "derod"}
	case "windows":
		possibleNames = []string{"derod-windows-amd64.exe", "derod.exe"}
	case "linux":
		switch runtime.GOARCH {
		case "arm64":
			possibleNames = []string{"derod-linux-arm64", "derod"}
		case "arm":
			possibleNames = []string{"derod-linux-arm", "derod"}
		default:
			possibleNames = []string{"derod-linux-amd64", "derod"}
		}
	default:
		possibleNames = []string{"derod"}
	}

	var found string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			for _, name := range possibleNames {
				if strings.EqualFold(info.Name(), name) {
					found = path
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	return found
}

// GetInstalledDerodPath returns the path to an installed derod, or empty if not found
func (d *DerodDownloader) GetInstalledDerodPath() string {
	// Look for any version directory
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		return ""
	}

	// Find the latest version (assuming semver-like naming)
	var latestVersion string
	var latestPath string

	for _, entry := range entries {
		if entry.IsDir() {
			versionDir := filepath.Join(d.baseDir, entry.Name())
			derodPath := d.findDerodBinary(versionDir)
			if derodPath != "" {
				// Use this if it's newer or the first one found
				if latestVersion == "" || entry.Name() > latestVersion {
					latestVersion = entry.Name()
					latestPath = derodPath
				}
			}
		}
	}

	return latestPath
}

// GetInstalledDerodVersion returns the version of the installed derod
func (d *DerodDownloader) GetInstalledDerodVersion() string {
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		return ""
	}

	var latestVersion string
	for _, entry := range entries {
		if entry.IsDir() {
			versionDir := filepath.Join(d.baseDir, entry.Name())
			if d.findDerodBinary(versionDir) != "" {
				if entry.Name() > latestVersion {
					latestVersion = entry.Name()
				}
			}
		}
	}

	return latestVersion
}

// IsDerodInstalled checks if derod is already installed
func (d *DerodDownloader) IsDerodInstalled() bool {
	return d.GetInstalledDerodPath() != ""
}

// DownloadLatestDerod downloads the latest derod from GitHub with checksum verification
func (d *DerodDownloader) DownloadLatestDerod() (string, error) {
	// Get latest release info
	release, err := d.GetLatestDeroRelease()
	if err != nil {
		return "", err
	}

	// Check if already installed
	currentVersion := d.GetInstalledDerodVersion()
	if currentVersion == release.TagName {
		d.app.logToConsole(fmt.Sprintf("[OK] derod %s is already installed", release.TagName))
		return d.GetInstalledDerodPath(), nil
	}

	// Fetch checksums first (mandatory for security)
	checksums, err := d.GetChecksumFile(release)
	if err != nil {
		return "", fmt.Errorf("failed to fetch checksums: %w - cannot verify download integrity", err)
	}

	// Find download URL for this platform
	url, _, err := d.FindAssetForPlatform(release)
	if err != nil {
		return "", err
	}

	// Get expected checksum for our platform's asset
	assetName := d.GetPlatformAssetName()
	expectedChecksum, hasChecksum := checksums[assetName]
	if !hasChecksum {
		return "", fmt.Errorf("no checksum found for %s - cannot verify download integrity", assetName)
	}

	d.app.logToConsole(fmt.Sprintf("[VERIFY] Expected SHA256: %s", expectedChecksum))

	// Download with checksum verification
	if err := d.DownloadDerod(url, release.TagName, expectedChecksum); err != nil {
		return "", err
	}

	return d.GetInstalledDerodPath(), nil
}

// --- Exposed methods for frontend ---

// CheckDerodStatus returns the current status of derod installation
func (a *App) CheckDerodStatus() map[string]interface{} {
	downloader := NewDerodDownloader(a)

	installed := downloader.IsDerodInstalled()
	version := downloader.GetInstalledDerodVersion()
	path := downloader.GetInstalledDerodPath()

	return map[string]interface{}{
		"installed": installed,
		"version":   version,
		"path":      path,
		"baseDir":   downloader.GetBaseDir(),
	}
}

// GetLatestDerodRelease fetches info about the latest DERO release
// Respects the allow_github_check setting
func (a *App) GetLatestDerodRelease() map[string]interface{} {
	// Check if GitHub checks are allowed
	if allowed, ok := a.settings["allow_github_check"].(bool); ok && !allowed {
		return map[string]interface{}{
			"success":        false,
			"error":          "GitHub checks are disabled in settings",
			"github_blocked": true,
		}
	}

	downloader := NewDerodDownloader(a)

	release, err := downloader.GetLatestDeroRelease()
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
	}

	// Find asset for current platform
	url, size, assetErr := downloader.FindAssetForPlatform(release)

	result := map[string]interface{}{
		"success":     true,
		"tagName":     release.TagName,
		"releaseName": release.Name,
		"platform":    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	if assetErr == nil {
		result["downloadUrl"] = url
		result["downloadSize"] = size
		result["downloadSizeMB"] = size / 1024 / 1024
	} else {
		result["assetError"] = assetErr.Error()
	}

	return result
}

// DownloadDerodFromGitHub downloads the latest derod from GitHub
// Respects the allow_github_check setting
func (a *App) DownloadDerodFromGitHub() map[string]interface{} {
	// Check if GitHub checks are allowed
	if allowed, ok := a.settings["allow_github_check"].(bool); ok && !allowed {
		a.logToConsole("[BLOCKED] GitHub checks disabled in settings")
		return map[string]interface{}{
			"success":        false,
			"error":          "GitHub checks are disabled. Please enable 'Allow GitHub Checks' in settings or install derod manually.",
			"github_blocked": true,
		}
	}

	a.logToConsole("[START] Starting derod download from GitHub...")

	downloader := NewDerodDownloader(a)
	path, err := downloader.DownloadLatestDerod()
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Download failed: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
	}

	a.logToConsole("[OK] derod downloaded and verified successfully")
	return map[string]interface{}{
		"success":  true,
		"path":     path,
		"version":  downloader.GetInstalledDerodVersion(),
		"verified": true,
	}
}

// GetManualDerodInstructions returns instructions for manual derod installation
func (a *App) GetManualDerodInstructions() map[string]interface{} {
	downloader := NewDerodDownloader(a)
	assetName := downloader.GetPlatformAssetName()

	instructions := fmt.Sprintf(`Manual derod Installation Instructions:

1. Download the latest DERO release from:
   https://github.com/deroproject/derohe/releases/latest

2. Download the file: %s

3. IMPORTANT: Also download 'checksum.txt.signed' and verify the SHA256 hash

4. Extract the archive and locate the derod binary

5. Place the binary in:
   %s/<version>/

6. Restart HOLOGRAM and it should detect the installed binary

Platform: %s/%s
Expected binary names: derod-darwin, derod-linux-amd64, derod-windows-amd64.exe`, 
		assetName,
		downloader.GetBaseDir(),
		runtime.GOOS,
		runtime.GOARCH,
	)

	return map[string]interface{}{
		"success":      true,
		"instructions": instructions,
		"downloadUrl":  "https://github.com/deroproject/derohe/releases/latest",
		"assetName":    assetName,
		"baseDir":      downloader.GetBaseDir(),
		"platform":     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// IsGitHubCheckAllowed returns whether GitHub checks are enabled
func (a *App) IsGitHubCheckAllowed() bool {
	if allowed, ok := a.settings["allow_github_check"].(bool); ok {
		return allowed
	}
	return true // Default to allowed
}
