package main

import (
	"fmt"
	"sync"

	"github.com/deroproject/graviton"
)

// NRSCache provides bidirectional name/address caching using Graviton
// This enables showing human-readable names alongside addresses in the Explorer
type NRSCache struct {
	store     *graviton.Store
	mu        sync.RWMutex
	app       *App
	cacheHits int64
	cacheMiss int64
}

// NewNRSCache creates a new NRS cache backed by Graviton
func NewNRSCache(dataDir string) *NRSCache {
	store, err := graviton.NewDiskStore(dataDir)
	if err != nil {
		// Fallback to in-memory store if disk fails
		store, _ = graviton.NewMemStore()
	}
	return &NRSCache{store: store}
}

// SetApp links the cache to app for live lookups
func (n *NRSCache) SetApp(app *App) {
	n.app = app
}

// CacheNameAddress stores a name<->address mapping bidirectionally
func (n *NRSCache) CacheNameAddress(name, address string) error {
	if name == "" || address == "" {
		return fmt.Errorf("name and address cannot be empty")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	ss, err := n.store.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// Store name → address
	nameTree, err := ss.GetTree("nrs_name_to_addr")
	if err != nil {
		return fmt.Errorf("failed to get name tree: %w", err)
	}
	if err := nameTree.Put([]byte(name), []byte(address)); err != nil {
		return fmt.Errorf("failed to put name: %w", err)
	}

	// Store address → name (reverse lookup)
	addrTree, err := ss.GetTree("nrs_addr_to_name")
	if err != nil {
		return fmt.Errorf("failed to get addr tree: %w", err)
	}
	if err := addrTree.Put([]byte(address), []byte(name)); err != nil {
		return fmt.Errorf("failed to put addr: %w", err)
	}

	_, err = graviton.Commit(nameTree, addrTree)
	return err
}

// GetNameForAddress returns the NRS name for an address (if cached)
func (n *NRSCache) GetNameForAddress(address string) (string, bool) {
	if address == "" {
		return "", false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	ss, err := n.store.LoadSnapshot(0)
	if err != nil {
		return "", false
	}

	tree, err := ss.GetTree("nrs_addr_to_name")
	if err != nil {
		return "", false
	}

	nameBytes, err := tree.Get([]byte(address))
	if err != nil || len(nameBytes) == 0 {
		n.cacheMiss++
		return "", false
	}

	n.cacheHits++
	return string(nameBytes), true
}

// GetAddressForName returns the address for a name (if cached)
func (n *NRSCache) GetAddressForName(name string) (string, bool) {
	if name == "" {
		return "", false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	ss, err := n.store.LoadSnapshot(0)
	if err != nil {
		return "", false
	}

	tree, err := ss.GetTree("nrs_name_to_addr")
	if err != nil {
		return "", false
	}

	addrBytes, err := tree.Get([]byte(name))
	if err != nil || len(addrBytes) == 0 {
		return "", false
	}

	return string(addrBytes), true
}

// LookupAndCache performs a live NRS lookup and caches the result
func (n *NRSCache) LookupAndCache(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name cannot be empty")
	}

	// Check cache first
	if addr, found := n.GetAddressForName(name); found {
		return addr, nil
	}

	// Do live lookup via app
	if n.app == nil {
		return "", fmt.Errorf("app not linked to cache")
	}

	result := n.app.ResolveDeroName(name)
	if success, ok := result["success"].(bool); ok && success {
		if addr, ok := result["address"].(string); ok && addr != "" {
			// Cache bidirectionally
			n.CacheNameAddress(name, addr)
			return addr, nil
		}
	}

	if errMsg, ok := result["error"].(string); ok {
		return "", fmt.Errorf(errMsg)
	}

	return "", fmt.Errorf("name not found")
}

// GetCacheStats returns cache statistics
func (n *NRSCache) GetCacheStats() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	total := n.cacheHits + n.cacheMiss
	ratio := float64(0)
	if total > 0 {
		ratio = float64(n.cacheHits) / float64(total)
	}

	// Count entries
	entryCount := int64(0)
	if ss, err := n.store.LoadSnapshot(0); err == nil {
		if tree, err := ss.GetTree("nrs_addr_to_name"); err == nil {
			c := tree.Cursor()
			for _, _, err := c.First(); err == nil; _, _, err = c.Next() {
				entryCount++
			}
		}
	}

	return map[string]interface{}{
		"hits":        n.cacheHits,
		"misses":      n.cacheMiss,
		"ratio":       ratio,
		"cachedNames": entryCount,
	}
}

// GetAllCachedNames returns all cached name→address mappings
func (n *NRSCache) GetAllCachedNames() map[string]string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make(map[string]string)

	ss, err := n.store.LoadSnapshot(0)
	if err != nil {
		return result
	}

	tree, err := ss.GetTree("nrs_name_to_addr")
	if err != nil {
		return result
	}

	c := tree.Cursor()
	for k, v, err := c.First(); err == nil; k, v, err = c.Next() {
		result[string(k)] = string(v)
	}

	return result
}

// ClearCache removes all cached entries
func (n *NRSCache) ClearCache() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Reset stats
	n.cacheHits = 0
	n.cacheMiss = 0

	// Clear trees by getting fresh snapshot and committing empty trees
	ss, err := n.store.LoadSnapshot(0)
	if err != nil {
		return err
	}

	nameTree, _ := ss.GetTree("nrs_name_to_addr")
	addrTree, _ := ss.GetTree("nrs_addr_to_name")

	// Delete all entries from name tree
	c := nameTree.Cursor()
	var keysToDelete [][]byte
	for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
		keysToDelete = append(keysToDelete, append([]byte{}, k...))
	}
	for _, k := range keysToDelete {
		nameTree.Delete(k)
	}

	// Delete all entries from addr tree
	c = addrTree.Cursor()
	keysToDelete = nil
	for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
		keysToDelete = append(keysToDelete, append([]byte{}, k...))
	}
	for _, k := range keysToDelete {
		addrTree.Delete(k)
	}

	_, err = graviton.Commit(nameTree, addrTree)
	return err
}

