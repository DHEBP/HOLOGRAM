package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/civilware/tela"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// FileService handles file operations

// GetFileInfo returns detailed information about a file
func (a *App) GetFileInfo(filePath string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[DOC] Getting file info: %s", filePath))

	info, err := os.Stat(filePath)
	if err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "File not found. Check the path.",
			"technicalError": err.Error(),
		}
	}

	// Get absolute path
	absPath, _ := filepath.Abs(filePath)

	// Detect MIME type
	ext := strings.ToLower(filepath.Ext(filePath))
	docType := tela.ParseDocType(filepath.Base(filePath))

	// Read first few bytes to detect content type
	contentPreview := ""
	if !info.IsDir() && info.Size() > 0 && info.Size() < 10*1024*1024 { // < 10MB
		data, err := os.ReadFile(filePath)
		if err == nil && len(data) > 0 {
			// Show first 500 bytes as preview for text files
			if strings.HasPrefix(docType, "text/") || docType == "application/javascript" || docType == "application/json" {
				previewLen := 500
				if len(data) < previewLen {
					previewLen = len(data)
				}
				contentPreview = string(data[:previewLen])
			}
		}
	}

	return map[string]interface{}{
		"success":      true,
		"name":         info.Name(),
		"path":         absPath,
		"size":         info.Size(),
		"isDir":        info.IsDir(),
		"modTime":      info.ModTime().Unix(),
		"extension":    ext,
		"docType":      docType,
		"preview":      contentPreview,
		"canCompress":  canCompress(docType),
		"gasEstimate":  estimateGasCost(int(info.Size())),
	}
}

// ShardFile splits a file into DocShards for TELA deployment
func (a *App) ShardFile(filePath string, compress bool) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("🔪 Sharding file: %s (compress: %v)", filePath, compress))

	// Check file exists
	info, err := os.Stat(filePath)
	if err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "File not found. Check the path.",
			"technicalError": err.Error(),
		}
	}

	if info.IsDir() {
		return map[string]interface{}{
			"success": false,
			"error":   "Cannot shard a directory",
		}
	}

	// Read file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ErrorResponse(err)
	}

	// Create output directory
	outputDir := filepath.Join(".", "datashards", "shards", filepath.Base(filePath))
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "Failed to create output directory",
			"technicalError": err.Error(),
		}
	}

	// Set shard path and create shards using tela library
	tela.SetShardPath(outputDir)
	
	compression := ""
	if compress {
		compression = tela.COMPRESSION_GZIP
	}
	
	err = tela.CreateShardFiles(filePath, compression, data)
	if err != nil {
		return ErrorResponse(err)
	}

	// Count shard files created
	totalShards, _ := tela.GetTotalShards(data)

	a.logToConsole(fmt.Sprintf("[OK] Created shard files in %s", outputDir))

	return map[string]interface{}{
		"success":     true,
		"shardCount":  totalShards,
		"outputDir":   outputDir,
		"compressed":  compress,
		"message":     fmt.Sprintf("File sharded into %d parts", totalShards),
	}
}

// ConstructFromShards reconstructs a file from DocShards
func (a *App) ConstructFromShards(shardPath string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[Shards] Constructing file from shards: %s", shardPath))

	// Check shard path exists
	info, err := os.Stat(shardPath)
	if err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "Shard path not found. Check the path.",
			"technicalError": err.Error(),
		}
	}

	var shardDir string
	if info.IsDir() {
		shardDir = shardPath
	} else {
		shardDir = filepath.Dir(shardPath)
	}

	// Find shard files in directory
	shardFiles := [][]byte{}
	err = filepath.Walk(shardDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, tela.TAG_DOC_SHARD) {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			shardFiles = append(shardFiles, data)
		}
		return nil
	})
	if err != nil {
		return ErrorResponse(err)
	}

	if len(shardFiles) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "No shard files found in directory",
		}
	}

	// Reconstruct using tela library
	outputPath := filepath.Join(shardDir, "reconstructed")
	err = tela.ConstructFromShards(shardFiles, outputPath, shardDir, "")
	if err != nil {
		return ErrorResponse(err)
	}

	// Get file size
	outputInfo, _ := os.Stat(outputPath)
	size := int64(0)
	if outputInfo != nil {
		size = outputInfo.Size()
	}

	a.logToConsole(fmt.Sprintf("[OK] File reconstructed: %s (%d bytes)", outputPath, size))

	return map[string]interface{}{
		"success":    true,
		"outputPath": outputPath,
		"size":       size,
		"message":    "File reconstructed successfully",
	}
}

// DiffFiles compares two files and returns the differences
func (a *App) DiffFiles(file1, file2 string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[STATS] Diffing files: %s vs %s", file1, file2))

	// Read file 1
	data1, err := os.ReadFile(file1)
	if err != nil {
		return ErrorResponse(err)
	}

	// Read file 2
	data2, err := os.ReadFile(file2)
	if err != nil {
		return ErrorResponse(err)
	}

	// Split into lines
	lines1 := strings.Split(string(data1), "\n")
	lines2 := strings.Split(string(data2), "\n")

	// Simple line-by-line diff
	diffs := computeLineDiff(lines1, lines2)

	return map[string]interface{}{
		"success":    true,
		"file1":      file1,
		"file2":      file2,
		"file1Lines": len(lines1),
		"file2Lines": len(lines2),
		"diffs":      diffs,
		"identical":  len(diffs) == 0,
	}
}

// DiffSCIDs compares the code of two smart contracts
func (a *App) DiffSCIDs(scid1, scid2 string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[STATS] Diffing SCIDs: %s vs %s", scid1[:16]+"...", scid2[:16]+"..."))

	// Get code for first SCID
	result1, err := a.daemonClient.Call("DERO.GetSC", map[string]interface{}{
		"scid": scid1,
		"code": true,
	})
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to get first SC: %v", err),
		}
	}

	// Get code for second SCID
	result2, err := a.daemonClient.Call("DERO.GetSC", map[string]interface{}{
		"scid": scid2,
		"code": true,
	})
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to get second SC: %v", err),
		}
	}

	// Extract code strings
	code1 := extractCode(result1)
	code2 := extractCode(result2)

	// Compute diff
	lines1 := strings.Split(code1, "\n")
	lines2 := strings.Split(code2, "\n")
	diffs := computeLineDiff(lines1, lines2)

	return map[string]interface{}{
		"success":    true,
		"scid1":      scid1,
		"scid2":      scid2,
		"code1Lines": len(lines1),
		"code2Lines": len(lines2),
		"diffs":      diffs,
		"identical":  len(diffs) == 0,
	}
}

// MoveFile moves a file or directory
func (a *App) MoveFile(source, destination string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[DIR] Moving: %s → %s", source, destination))

	// Check source exists
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return map[string]interface{}{
			"success": false,
			"error":   "Source file not found",
		}
	}

	// Create destination directory if needed
	destDir := filepath.Dir(destination)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create destination directory: %v", err),
		}
	}

	// Move the file
	if err := os.Rename(source, destination); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to move file: %v", err),
		}
	}

	a.logToConsole(fmt.Sprintf("[OK] File moved to: %s", destination))

	return map[string]interface{}{
		"success":     true,
		"source":      source,
		"destination": destination,
		"message":     "File moved successfully",
	}
}

// RemoveFile removes a file or directory (only from datashards/clone)
func (a *App) RemoveFile(path string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("🗑️ Removing: %s", path))

	// Security check: only allow removal from datashards directory
	absPath, _ := filepath.Abs(path)
	shardsDir, _ := filepath.Abs(filepath.Join(".", "datashards"))
	
	if !strings.HasPrefix(absPath, shardsDir) {
		return map[string]interface{}{
			"success": false,
			"error":   "Can only remove files from datashards directory",
		}
	}

	// Check file exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return map[string]interface{}{
			"success": false,
			"error":   "File not found",
		}
	}

	// Remove file or directory
	if info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Failed to remove directory: %v", err),
			}
		}
	} else {
		if err := os.Remove(path); err != nil {
			return map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Failed to remove file: %v", err),
			}
		}
	}

	a.logToConsole(fmt.Sprintf("[OK] Removed: %s", path))

	return map[string]interface{}{
		"success": true,
		"path":    path,
		"message": "File removed successfully",
	}
}

// ListDirectory lists contents of a directory
func (a *App) ListDirectory(dirPath string) map[string]interface{} {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to read directory: %v", err),
		}
	}

	items := []map[string]interface{}{}
	for _, entry := range entries {
		info, _ := entry.Info()
		items = append(items, map[string]interface{}{
			"name":    entry.Name(),
			"isDir":   entry.IsDir(),
			"size":    info.Size(),
			"modTime": info.ModTime().Unix(),
		})
	}

	return map[string]interface{}{
		"success": true,
		"path":    dirPath,
		"items":   items,
		"count":   len(items),
	}
}

// ================== FOLDER SELECTION ==================

// SelectFolder opens a native directory picker dialog
func (a *App) SelectFolder() string {
	selection, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Select Folder for TELA Deployment",
		CanCreateDirectories: false,
	})
	if err != nil {
		log.Printf("Error opening directory dialog: %v", err)
		return ""
	}
	return selection
}

// SelectFile opens a native file picker dialog
func (a *App) SelectFile() string {
	selection, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select File",
	})
	if err != nil {
		log.Printf("Error opening file dialog: %v", err)
		return ""
	}
	return selection
}

// SelectFiles opens a native multiple file picker dialog for TELA DOC uploads
func (a *App) SelectFiles() map[string]interface{} {
	a.logToConsole("[FILE] SelectFiles: Opening native file dialog...")
	
	selections, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Files for TELA Upload",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Web Files",
				Pattern:     "*.html;*.htm;*.css;*.js;*.json;*.svg;*.png;*.jpg;*.jpeg;*.gif;*.webp;*.ico;*.woff;*.woff2;*.ttf",
			},
			{
				DisplayName: "All Files",
				Pattern:     "*.*",
			},
		},
	})
	
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] SelectFiles: Dialog error - %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
	}

	a.logToConsole(fmt.Sprintf("[FILE] SelectFiles: Dialog returned %d selections", len(selections)))
	
	if len(selections) == 0 {
		a.logToConsole("[FILE] SelectFiles: No files selected (user cancelled)")
		return map[string]interface{}{
			"success": false,
			"error":   "No files selected",
		}
	}

	// Process selected files
	a.logToConsole("[FILE] SelectFiles: Processing selected files...")
	var files []map[string]interface{}
	for _, filePath := range selections {
		info, err := os.Stat(filePath)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] SelectFiles: Could not stat %s - %v", filePath, err))
			continue
		}

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] SelectFiles: Could not read %s - %v", filePath, err))
			continue
		}

		files = append(files, map[string]interface{}{
			"name":    info.Name(),
			"path":    filePath,
			"subDir":  "/",
			"size":    info.Size(),
			"type":    detectMimeType(info.Name()),
			"data":    string(content),
		})
		a.logToConsole(fmt.Sprintf("[FILE] SelectFiles: Loaded %s (%d bytes)", info.Name(), info.Size()))
	}

	a.logToConsole(fmt.Sprintf("[OK] SelectFiles: Returning %d files", len(files)))
	return map[string]interface{}{
		"success": true,
		"files":   files,
	}
}

// detectMimeType returns the MIME type based on file extension
func detectMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimeTypes := map[string]string{
		".html":  "text/html",
		".htm":   "text/html",
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".svg":   "image/svg+xml",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".gif":   "image/gif",
		".webp":  "image/webp",
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".ico":   "image/x-icon",
	}
	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// ================== BATCH UPLOAD (Folder Scanner) ==================

// FolderScanResult represents the result of scanning a folder
type FolderScanResult struct {
	Files       []ScannedFile `json:"files"`
	TotalFiles  int           `json:"totalFiles"`
	TotalSize   int64         `json:"totalSize"`
	TotalGas    uint64        `json:"totalGas"`
	FolderPath  string        `json:"folderPath"`
	Errors      []string      `json:"errors"`
}

// ScannedFile represents a file found during folder scanning
type ScannedFile struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelPath      string `json:"relPath"`
	SubDir       string `json:"subDir"`
	DocType      string `json:"docType"`
	Size         int64  `json:"size"`
	IsEntryPoint bool   `json:"isEntryPoint"` // e.g., index.html
	CanCompress  bool   `json:"canCompress"`
	GasEstimate  uint64 `json:"gasEstimate"`
}

// ScanFolder recursively scans a folder for TELA deployment
func (a *App) ScanFolder(folderPath string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[DIR] Scanning folder for TELA deployment: %s", folderPath))

	// Validate folder exists
	info, err := os.Stat(folderPath)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Folder not found: %v", err),
		}
	}
	if !info.IsDir() {
		return map[string]interface{}{
			"success": false,
			"error":   "Path is not a directory",
		}
	}

	result := FolderScanResult{
		Files:      []ScannedFile{},
		FolderPath: folderPath,
		Errors:     []string{},
	}

	// Walk the folder
	err = filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Error accessing %s: %v", path, err))
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip hidden files and common exclusions
		name := info.Name()
		if strings.HasPrefix(name, ".") || name == "Thumbs.db" || name == ".DS_Store" {
			return nil
		}

		// Calculate relative path and subDir
		relPath, _ := filepath.Rel(folderPath, path)
		subDir := filepath.Dir(relPath)
		if subDir == "." {
			subDir = "/"
		} else {
			subDir = "/" + filepath.ToSlash(subDir)
		}

		// Detect document type
		docType := tela.ParseDocType(name)

		// Check if this is an entry point
		isEntry := strings.ToLower(name) == "index.html" && (subDir == "/" || subDir == "")

		file := ScannedFile{
			Name:         name,
			Path:         path,
			RelPath:      relPath,
			SubDir:       subDir,
			DocType:      docType,
			Size:         info.Size(),
			IsEntryPoint: isEntry,
			CanCompress:  canCompress(docType),
			GasEstimate:  estimateGasCost(int(info.Size())),
		}

		result.Files = append(result.Files, file)
		result.TotalSize += info.Size()
		result.TotalGas += file.GasEstimate

		return nil
	})

	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Walk error: %v", err))
	}

	result.TotalFiles = len(result.Files)

	a.logToConsole(fmt.Sprintf("[OK] Scanned %d files (%.2f KB, ~%d gas)", 
		result.TotalFiles, 
		float64(result.TotalSize)/1024, 
		result.TotalGas))

	return map[string]interface{}{
		"success":    true,
		"files":      result.Files,
		"totalFiles": result.TotalFiles,
		"totalSize":  result.TotalSize,
		"totalGas":   result.TotalGas,
		"folderPath": result.FolderPath,
		"errors":     result.Errors,
	}
}

// GenerateSubDirs generates subDir paths from a list of files
func (a *App) GenerateSubDirs(folderPath string, filesJSON string) map[string]interface{} {
	// Parse files array
	var files []map[string]interface{}
	if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid files JSON",
		}
	}

	// Generate subDirs
	result := []map[string]interface{}{}
	for _, f := range files {
		path, ok := f["path"].(string)
		if !ok {
			continue
		}

		relPath, _ := filepath.Rel(folderPath, path)
		subDir := filepath.Dir(relPath)
		if subDir == "." {
			subDir = "/"
		} else {
			subDir = "/" + filepath.ToSlash(subDir)
		}

		f["subDir"] = subDir
		result = append(result, f)
	}

	return map[string]interface{}{
		"success": true,
		"files":   result,
	}
}

// DetectDocTypes updates doc types for a list of files
func (a *App) DetectDocTypes(filesJSON string) map[string]interface{} {
	var files []map[string]interface{}
	if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid files JSON",
		}
	}

	for i := range files {
		if name, ok := files[i]["name"].(string); ok {
			files[i]["docType"] = tela.ParseDocType(name)
		}
	}

	return map[string]interface{}{
		"success": true,
		"files":   files,
	}
}

// EstimateBatchGas calculates total gas for a batch of files
func (a *App) EstimateBatchGas(filesJSON string) map[string]interface{} {
	var files []map[string]interface{}
	if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid files JSON",
		}
	}

	var totalGas uint64
	var totalSize int64

	for _, f := range files {
		size, ok := f["size"].(float64)
		if ok {
			totalSize += int64(size)
			totalGas += estimateGasCost(int(size))
		}
	}

	// Add INDEX creation cost
	indexGas := uint64(10000) // Base cost for INDEX

	return map[string]interface{}{
		"success":   true,
		"docsGas":   totalGas,
		"indexGas":  indexGas,
		"totalGas":  totalGas + indexGas,
		"totalSize": totalSize,
		"fileCount": len(files),
	}
}

// Helper functions

func extractCode(result interface{}) string {
	if resultMap, ok := result.(map[string]interface{}); ok {
		if code, ok := resultMap["code"].(string); ok {
			return code
		}
	}
	return ""
}

// DiffLine represents a single line difference
type DiffLine struct {
	LineNum int    `json:"lineNum"`
	Type    string `json:"type"` // "add", "remove", "change"
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
}

func computeLineDiff(lines1, lines2 []string) []DiffLine {
	diffs := []DiffLine{}

	// Simple line-by-line comparison
	// For more sophisticated diff, use a proper diff library
	maxLen := len(lines1)
	if len(lines2) > maxLen {
		maxLen = len(lines2)
	}

	for i := 0; i < maxLen; i++ {
		var line1, line2 string
		
		if i < len(lines1) {
			line1 = lines1[i]
		}
		if i < len(lines2) {
			line2 = lines2[i]
		}

		if line1 != line2 {
			diff := DiffLine{
				LineNum: i + 1,
			}
			
			if line1 == "" {
				diff.Type = "add"
				diff.New = line2
			} else if line2 == "" {
				diff.Type = "remove"
				diff.Old = line1
			} else {
				diff.Type = "change"
				diff.Old = line1
				diff.New = line2
			}
			
			diffs = append(diffs, diff)
		}
	}

	return diffs
}

