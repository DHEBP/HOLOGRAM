// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Gnomon Historical Variable Queries
// Ported from simple-gnomon for time-travel SC state queries

package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Default and maximum snapshot limits
const (
	DefaultMaxSnapshots = 100
	AbsoluteMaxSnapshots = 1000
)

// VariableSnapshot stores variables at a specific height
type VariableSnapshot struct {
	Height    int64                  `json:"height"`
	Variables map[string]interface{} `json:"variables"`
}

// SCIDHistory stores variable history for a SCID
type SCIDHistory struct {
	SCID      string             `json:"scid"`
	Snapshots []VariableSnapshot `json:"snapshots"`
}

// VariableHistoryStore manages variable history
type VariableHistoryStore struct {
	History      map[string]*SCIDHistory `json:"history"` // SCID -> history
	MaxSnapshots int                     `json:"max_snapshots"`
	mu           sync.RWMutex
	filePath     string
}

var varHistoryStore *VariableHistoryStore
var varHistoryStoreOnce sync.Once

// InitVariableHistoryStore initializes the history store (singleton)
func InitVariableHistoryStore() *VariableHistoryStore {
	varHistoryStoreOnce.Do(func() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[HISTORY] Failed to get home directory: %v", err)
			homeDir = "."
		}

		basePath := filepath.Join(homeDir, ".dero", "hologram", "datashards")

		varHistoryStore = &VariableHistoryStore{
			History:      make(map[string]*SCIDHistory),
			MaxSnapshots: DefaultMaxSnapshots,
			filePath:     filepath.Join(basePath, "variable_history.json"),
		}

		varHistoryStore.load()
		log.Printf("[HISTORY] Variable history store initialized with %d SCIDs", len(varHistoryStore.History))
	})

	return varHistoryStore
}

// SetMaxSnapshots sets the maximum number of snapshots to keep per SCID
func (s *VariableHistoryStore) SetMaxSnapshots(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if max < 10 {
		max = 10 // Minimum 10 snapshots
	}
	if max > AbsoluteMaxSnapshots {
		max = AbsoluteMaxSnapshots
	}

	s.MaxSnapshots = max
	log.Printf("[HISTORY] Max snapshots set to %d", max)
}

// GetMaxSnapshots returns the current max snapshots setting
func (s *VariableHistoryStore) GetMaxSnapshots() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MaxSnapshots
}

// RecordSnapshot records a variable snapshot at a specific height
func (s *VariableHistoryStore) RecordSnapshot(scid string, height int64, vars map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.History[scid] == nil {
		s.History[scid] = &SCIDHistory{
			SCID:      scid,
			Snapshots: make([]VariableSnapshot, 0),
		}
	}

	// Check if we already have this height
	for _, snap := range s.History[scid].Snapshots {
		if snap.Height == height {
			return // Already recorded
		}
	}

	// Deep copy the variables to avoid mutation
	varsCopy := make(map[string]interface{})
	for k, v := range vars {
		varsCopy[k] = v
	}

	s.History[scid].Snapshots = append(s.History[scid].Snapshots, VariableSnapshot{
		Height:    height,
		Variables: varsCopy,
	})

	// Sort by height (ascending)
	sort.Slice(s.History[scid].Snapshots, func(i, j int) bool {
		return s.History[scid].Snapshots[i].Height < s.History[scid].Snapshots[j].Height
	})

	// Trim to max snapshots (keep most recent)
	if len(s.History[scid].Snapshots) > s.MaxSnapshots {
		excess := len(s.History[scid].Snapshots) - s.MaxSnapshots
		s.History[scid].Snapshots = s.History[scid].Snapshots[excess:]
	}

	// Save asynchronously
	go s.save()
}

// GetVariablesAtHeight returns variables at or before the specified height
func (s *VariableHistoryStore) GetVariablesAtHeight(scid string, height int64) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.History[scid]
	if history == nil || len(history.Snapshots) == 0 {
		return nil
	}

	// Find the snapshot at or before the requested height
	var result map[string]interface{}
	for _, snap := range history.Snapshots {
		if snap.Height <= height {
			result = snap.Variables
		} else {
			break
		}
	}

	return result
}

// GetVariablesAtExactHeight returns variables at exactly the specified height (or nil)
func (s *VariableHistoryStore) GetVariablesAtExactHeight(scid string, height int64) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.History[scid]
	if history == nil {
		return nil
	}

	for _, snap := range history.Snapshots {
		if snap.Height == height {
			return snap.Variables
		}
	}

	return nil
}

// GetInteractionHeights returns all heights where this SCID was recorded
func (s *VariableHistoryStore) GetInteractionHeights(scid string) []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.History[scid]
	if history == nil {
		return []int64{}
	}

	heights := make([]int64, len(history.Snapshots))
	for i, snap := range history.Snapshots {
		heights[i] = snap.Height
	}
	return heights
}

// GetSnapshotCount returns the number of snapshots for a SCID
func (s *VariableHistoryStore) GetSnapshotCount(scid string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.History[scid]
	if history == nil {
		return 0
	}
	return len(history.Snapshots)
}

// GetAllSCIDs returns all SCIDs with recorded history
func (s *VariableHistoryStore) GetAllSCIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	scids := make([]string, 0, len(s.History))
	for scid := range s.History {
		scids = append(scids, scid)
	}
	return scids
}

// GetStats returns statistics about the history store
func (s *VariableHistoryStore) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalSnapshots := 0
	for _, h := range s.History {
		totalSnapshots += len(h.Snapshots)
	}

	return map[string]interface{}{
		"total_scids":     len(s.History),
		"total_snapshots": totalSnapshots,
		"max_snapshots":   s.MaxSnapshots,
	}
}

// ClearSCID removes all history for a specific SCID
func (s *VariableHistoryStore) ClearSCID(scid string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.History, scid)
	go s.save()
}

// ClearAll clears all history
func (s *VariableHistoryStore) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.History = make(map[string]*SCIDHistory)
	go s.save()

	log.Printf("[HISTORY] Variable history cleared")
}

// CompareHeights compares variables between two heights and returns the differences
func (s *VariableHistoryStore) CompareHeights(scid string, height1, height2 int64) map[string]interface{} {
	vars1 := s.GetVariablesAtHeight(scid, height1)
	vars2 := s.GetVariablesAtHeight(scid, height2)

	if vars1 == nil && vars2 == nil {
		return map[string]interface{}{
			"error": "No data available for either height",
		}
	}

	if vars1 == nil {
		vars1 = make(map[string]interface{})
	}
	if vars2 == nil {
		vars2 = make(map[string]interface{})
	}

	// Find added, removed, and changed keys
	added := make(map[string]interface{})
	removed := make(map[string]interface{})
	changed := make(map[string]interface{})
	unchanged := make([]string, 0)

	// Check for removed and changed
	for k, v1 := range vars1 {
		if v2, exists := vars2[k]; exists {
			// Key exists in both - check if changed
			if !valuesEqual(v1, v2) {
				changed[k] = map[string]interface{}{
					"from": v1,
					"to":   v2,
				}
			} else {
				unchanged = append(unchanged, k)
			}
		} else {
			// Key removed
			removed[k] = v1
		}
	}

	// Check for added
	for k, v2 := range vars2 {
		if _, exists := vars1[k]; !exists {
			added[k] = v2
		}
	}

	return map[string]interface{}{
		"height1":   height1,
		"height2":   height2,
		"added":     added,
		"removed":   removed,
		"changed":   changed,
		"unchanged": unchanged,
	}
}

// valuesEqual compares two interface{} values
func valuesEqual(a, b interface{}) bool {
	// Simple comparison - could be enhanced for deep comparison
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

// load reads history from disk
func (s *VariableHistoryStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[HISTORY] Failed to load history store: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, s); err != nil {
		log.Printf("[HISTORY] Failed to parse history store: %v", err)
	}
}

// save writes history to disk
func (s *VariableHistoryStore) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[HISTORY] Failed to create directory: %v", err)
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		log.Printf("[HISTORY] Failed to marshal history store: %v", err)
		return err
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		log.Printf("[HISTORY] Failed to write history store: %v", err)
		return err
	}

	return nil
}

