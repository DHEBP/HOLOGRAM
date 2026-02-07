package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/deroproject/graviton"
)

// ContentCache defines a minimal cache API for TELA INDEX/DOC content.
type ContentCache interface {
    PutHTML(scid string, html string) error
    GetHTML(scid string) (string, bool)
    PutHTMLVersion(scid string, version int64, html string) error
    GetHTMLIfVersion(scid string, version int64) (string, bool)
    PutHTMLVersionHash(scid string, version int64, hash string, html string) error
    PutHTMLVersionHashWithDURL(scid string, durl string, version int64, hash string, html string) error
    GetHTMLIfVersionByDURL(durl string, version int64) (string, bool)
}

// GravitonCache implements ContentCache backed by a graviton disk store.
type GravitonCache struct {
    store   *graviton.Store
    tree    string
}

// NewGravitonCache initializes a disk-backed graviton store under datashards/cache.
func NewGravitonCache() *GravitonCache {
    // Determine cache path alongside other datashards
	cachePath := filepath.Join(getDatashardsDir(), "tela_cache")
    _ = os.MkdirAll(cachePath, 0755)

    // Initialize disk store
    st, err := graviton.NewDiskStore(cachePath)
    if err != nil {
        // Fallback to in-memory store to avoid hard failure
        st, _ = graviton.NewMemStore()
    }

    return &GravitonCache{store: st, tree: "tela_cache_html"}
}

func (c *GravitonCache) PutHTML(scid string, html string) error {
    // Delegate to versioned put with version 0 (unknown)
    return c.PutHTMLVersion(scid, 0, html)
}

func (c *GravitonCache) GetHTML(scid string) (string, bool) {
    // Ignore version; return whatever exists
    if c.store == nil { return "", false }
    ss, err := c.store.LoadSnapshot(0); if err != nil { return "", false }
    tree, _ := ss.GetTree(c.tree); if tree == nil { return "", false }
    v, _ := tree.Get([]byte(scid)); if v == nil { return "", false }
    s := string(v)
    if html, ok := extractHTMLFromPayload(s); ok {
        return html, true
    }
    return s, true
}

// PutHTMLVersion stores an entry tagged by a version (e.g., latest interaction height)
func (c *GravitonCache) PutHTMLVersion(scid string, version int64, html string) error {
    if c.store == nil {
        return fmt.Errorf("cache store not initialized")
    }
    ss, err := c.store.LoadSnapshot(0)
    if err != nil {
        return err
    }
    tree, _ := ss.GetTree(c.tree)
    // Store JSON payload {"v":version, "h":html}
    payload := fmt.Sprintf("{\"v\":%d,\"h\":%q}", version, html)
    if err := tree.Put([]byte(scid), []byte(payload)); err != nil {
        return err
    }
    _, err = graviton.Commit(tree)
    return err
}

// PutHTMLVersionHash stores version + content hash + html
func (c *GravitonCache) PutHTMLVersionHash(scid string, version int64, hash string, html string) error {
    if c.store == nil {
        return fmt.Errorf("cache store not initialized")
    }
    ss, err := c.store.LoadSnapshot(0)
    if err != nil {
        return err
    }
    tree, _ := ss.GetTree(c.tree)
    // Store JSON payload {"v":version, "x":hash, "h":html}
    payload := fmt.Sprintf("{\"v\":%d,\"x\":%q,\"h\":%q}", version, hash, html)
    if err := tree.Put([]byte(scid), []byte(payload)); err != nil {
        return err
    }
    _, err = graviton.Commit(tree)
    return err
}

// PutHTMLVersionHashWithDURL stores payload under SCID key and a secondary dURL key
func (c *GravitonCache) PutHTMLVersionHashWithDURL(scid string, durl string, version int64, hash string, html string) error {
    if err := c.PutHTMLVersionHash(scid, version, hash, html); err != nil { return err }
    if durl == "" { return nil }
    if c.store == nil { return fmt.Errorf("cache store not initialized") }
    ss, err := c.store.LoadSnapshot(0); if err != nil { return err }
    tree, _ := ss.GetTree(c.tree)
    key := "durl::" + durl
    payload := fmt.Sprintf("{\"v\":%d,\"x\":%q,\"h\":%q}", version, hash, html)
    if err := tree.Put([]byte(key), []byte(payload)); err != nil { return err }
    _, err = graviton.Commit(tree)
    return err
}

func (c *GravitonCache) GetHTMLIfVersionByDURL(durl string, version int64) (string, bool) {
    if c.store == nil { return "", false }
    ss, err := c.store.LoadSnapshot(0); if err != nil { return "", false }
    tree, _ := ss.GetTree(c.tree); if tree == nil { return "", false }
    key := "durl::" + durl
    v, _ := tree.Get([]byte(key)); if v == nil { return "", false }
    // Reuse version check from GetHTMLIfVersion path by parsing JSON minimally
    s := string(v)
    // extract version
    var storedV int64 = -1
    if idx := indexOf(s, "\"v\":"); idx >= 0 {
        i := idx + 5; j := i
        for j < len(s) && (s[j] == '-' || (s[j] >= '0' && s[j] <= '9')) { j++ }
        if i < j {
            var val int64 = 0; sign := int64(1); k := i
            if s[k] == '-' { sign = -1; k++ }
            for ; k < j; k++ { val = val*10 + int64(s[k]-'0') }
            storedV = val * sign
        }
    }
    if storedV != version { return "", false }
    if idx := indexOf(s, "\"h\":"); idx >= 0 {
        i := idx + 5
        for i < len(s) && s[i] != '"' { i++ }
        if i < len(s) && s[i] == '"' {
            i++
            j := i; esc := false
            for j < len(s) {
                if s[j] == '"' && !esc { break }
                if s[j] == '\\' && !esc { esc = true } else { esc = false }
                j++
            }
            if j <= len(s) {
                raw := s[i:j]
                raw = unescapeJSON(raw)
                return raw, true
            }
        }
    }
    return "", false
}

// GetHTMLIfVersion returns html only if the stored version matches requested.
func (c *GravitonCache) GetHTMLIfVersion(scid string, version int64) (string, bool) {
    if c.store == nil {
        return "", false
    }
    ss, err := c.store.LoadSnapshot(0)
    if err != nil {
        return "", false
    }
    tree, _ := ss.GetTree(c.tree)
    if tree == nil {
        return "", false
    }
    v, _ := tree.Get([]byte(scid))
    if v == nil {
        return "", false
    }
    // Expect JSON {"v":<int>,"h":"..."}
    // Very lightweight parse without a full JSON unmarshal to avoid new deps here
    s := string(v)
    // find "v": and "h":
    // naive parse:
    var storedV int64 = -1
    // try to locate "v":
    if idx := indexOf(s, "\"v\":"); idx >= 0 {
        // parse number starting after idx+5
        i := idx + 5
        j := i
        for j < len(s) && (s[j] == '-' || (s[j] >= '0' && s[j] <= '9')) { j++ }
        if i < j {
            // parse int64
            var val int64 = 0
            sign := int64(1)
            k := i
            if s[k] == '-' { sign = -1; k++ }
            for ; k < j; k++ { val = val*10 + int64(s[k]-'0') }
            storedV = val * sign
        }
    }
    if storedV != version {
        return "", false
    }
    if idx := indexOf(s, "\"h\":"); idx >= 0 {
        // value is a JSON string; we can return raw slice but it includes quotes and escapes.
        // For simplicity here, return substring unquoted best-effort.
        i := idx + 5
        if i < len(s) && s[i-1] == ':' {
            // find opening quote
            for i < len(s) && s[i] != '"' { i++ }
            if i < len(s) && s[i] == '"' {
                i++
                // find closing quote not preceded by backslash (naive)
                j := i
                esc := false
                for j < len(s) {
                    if s[j] == '"' && !esc { break }
                    if s[j] == '\\' && !esc { esc = true } else { esc = false }
                    j++
                }
                if j <= len(s) {
                    raw := s[i:j]
                    // unescape minimal sequences
                    raw = unescapeJSON(raw)
                    return raw, true
                }
            }
        }
    }
    return "", false
}

// PutDURLMapping stores a normalized dURL -> SCID mapping for fast resolution.
func (c *GravitonCache) PutDURLMapping(durl string, scid string) error {
    if c.store == nil {
        return fmt.Errorf("cache store not initialized")
    }
    durl = normalizeDURL(durl)
    if durl == "" || scid == "" {
        return nil
    }
    ss, err := c.store.LoadSnapshot(0)
    if err != nil {
        return err
    }
    tree, _ := ss.GetTree("durl_cache")
    if err := tree.Put([]byte(durl), []byte(scid)); err != nil {
        return err
    }
    _, err = graviton.Commit(tree)
    return err
}

// GetDURLMapping loads a normalized dURL -> SCID mapping.
func (c *GravitonCache) GetDURLMapping(durl string) (string, bool) {
    if c.store == nil {
        return "", false
    }
    durl = normalizeDURL(durl)
    if durl == "" {
        return "", false
    }
    ss, err := c.store.LoadSnapshot(0)
    if err != nil {
        return "", false
    }
    tree, _ := ss.GetTree("durl_cache")
    if tree == nil {
        return "", false
    }
    v, _ := tree.Get([]byte(durl))
    if v == nil {
        return "", false
    }
    return string(v), true
}

// indexOf returns the first index of substr in s or -1
func indexOf(s, substr string) int {
    // naive search to avoid importing strings again (already used elsewhere but keep local helpers)
    for i := 0; i+len(substr) <= len(s); i++ {
        if s[i:i+len(substr)] == substr {
            return i
        }
    }
    return -1
}

// unescapeJSON handles minimal escape sequences used in our payload (\\" and \\n)
func unescapeJSON(s string) string {
    // replace \\" with ", \\n with newline, \\t with tab, \\\\ with \\
    out := make([]byte, 0, len(s))
    for i := 0; i < len(s); i++ {
        if s[i] == '\\' && i+1 < len(s) {
            switch s[i+1] {
            case '"': out = append(out, '"'); i++
            case '\\': out = append(out, '\\'); i++
            case 'n': out = append(out, '\n'); i++
            case 't': out = append(out, '\t'); i++
            case 'r': out = append(out, '\r'); i++
            default: out = append(out, s[i])
            }
        } else {
            out = append(out, s[i])
        }
    }
    return string(out)
}

func extractHTMLFromPayload(payload string) (string, bool) {
    if idx := indexOf(payload, "\"h\":"); idx >= 0 {
        i := idx + 5
        for i < len(payload) && payload[i] != '"' { i++ }
        if i < len(payload) && payload[i] == '"' {
            i++
            j := i
            esc := false
            for j < len(payload) {
                if payload[j] == '"' && !esc { break }
                if payload[j] == '\\' && !esc { esc = true } else { esc = false }
                j++
            }
            if j <= len(payload) {
                raw := payload[i:j]
                raw = unescapeJSON(raw)
                return raw, true
            }
        }
    }
    return "", false
}

func normalizeDURL(durl string) string {
    durl = strings.ToLower(strings.TrimSpace(durl))
    if strings.HasPrefix(durl, "dero://") {
        durl = durl[7:]
    }
    return durl
}

func (a *App) cacheDURLMapping(durl string, scid string) {
    if a == nil || a.cache == nil {
        return
    }
    if gc, ok := a.cache.(*GravitonCache); ok {
        _ = gc.PutDURLMapping(durl, scid)
    }
}

func (a *App) getCachedDURLMapping(durl string) (string, bool) {
    if a == nil || a.cache == nil {
        return "", false
    }
    if gc, ok := a.cache.(*GravitonCache); ok {
        return gc.GetDURLMapping(durl)
    }
    return "", false
}

// ---------------- Inverted Index Persistence (ftindex) -----------------

func persistInvertedIndex(a *App, tokDURL, tokName, tokDesc map[string]map[string]struct{}, lastHeight int64) {
    if a == nil || a.cache == nil { return }
    gc, ok := a.cache.(*GravitonCache)
    if !ok || gc.store == nil { return }
    ss, err := gc.store.LoadSnapshot(0); if err != nil { return }
    tree, _ := ss.GetTree("ftindex")
    // store postings as CSV under keys per field
    for tok, set := range tokDURL { _ = tree.Put([]byte("tokd::"+tok), []byte(makeCSV(set))) }
    for tok, set := range tokName { _ = tree.Put([]byte("tokn::"+tok), []byte(makeCSV(set))) }
    for tok, set := range tokDesc { _ = tree.Put([]byte("tokx::"+tok), []byte(makeCSV(set))) }
    // store meta last height
    _ = tree.Put([]byte("meta::last_height"), []byte(fmt.Sprintf("%d", lastHeight)))
    _, _ = graviton.Commit(tree)
}

func loadInvertedPostingKind(a *App, token string, kind rune) []string {
    gc, ok := a.cache.(*GravitonCache)
    if !ok || gc.store == nil { return nil }
    ss, err := gc.store.LoadSnapshot(0); if err != nil { return nil }
    tree, _ := ss.GetTree("ftindex"); if tree == nil { return nil }
    var prefix string
    switch kind { case 'd': prefix = "tokd::"; case 'n': prefix = "tokn::"; default: prefix = "tokx::" }
    v, _ := tree.Get([]byte(prefix + token)); if v == nil { return nil }
    return splitCSV(string(v))
}

func loadFTLastHeight(a *App) int64 {
    gc, ok := a.cache.(*GravitonCache)
    if !ok || gc.store == nil { return 0 }
    ss, err := gc.store.LoadSnapshot(0); if err != nil { return 0 }
    tree, _ := ss.GetTree("ftindex"); if tree == nil { return 0 }
    v, _ := tree.Get([]byte("meta::last_height")); if v == nil { return 0 }
    // parse int64
    s := string(v)
    var n int64 = 0
    sign := int64(1)
    i := 0
    if len(s) > 0 && s[0] == '-' { sign = -1; i++ }
    for ; i < len(s); i++ { if s[i] < '0' || s[i] > '9' { break }; n = n*10 + int64(s[i]-'0') }
    return n * sign
}

func makeCSV(set map[string]struct{}) string {
    // deterministic order not required; simple join
    out := make([]byte, 0, 64*len(set))
    first := true
    for sc := range set {
        if !first { out = append(out, ',') } else { first = false }
        out = append(out, sc...)
    }
    return string(out)
}

func splitCSV(s string) []string {
    if s == "" { return nil }
    // no escaping expectations; SCIDs are hex
    parts := []string{}
    start := 0
    for i := 0; i < len(s); i++ {
        if s[i] == ',' {
            if i > start { parts = append(parts, s[start:i]) }
            start = i+1
        }
    }
    if start < len(s) { parts = append(parts, s[start:]) }
    return parts
}


