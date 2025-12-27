package main

import (
    "regexp"
    "sort"
    "strconv"
    "strings"
)

// TextIndex is a simple inverted index persisted via graviton
type TextIndex struct{}

var wordRE = regexp.MustCompile(`[A-Za-z0-9_]+`)

// BuildTextIndex builds an inverted index token->set(SCID) from Gnomon apps
// and stores it in graviton tree "ftindex" as entries: key "tok::<token>" -> CSV of SCIDs
func (a *App) BuildTextIndex() {
    if a.gnomonClient == nil || !a.gnomonClient.IsRunning() || a.cache == nil {
        return
    }
    // Guard: build only when Gnomon advances
    current := a.gnomonClient.Indexer.LastIndexedHeight
    last := loadFTLastHeight(a)
    if current <= last {
        return
    }

    apps := a.gnomonClient.GetTELAApps()
    tokDURL := make(map[string]map[string]struct{}) // dURL tokens
    tokName := make(map[string]map[string]struct{}) // name/display tokens
    tokDesc := make(map[string]map[string]struct{}) // description tokens
    add := func(m map[string]map[string]struct{}, tok, scid string) {
        if tok == "" || scid == "" { return }
        t := strings.ToLower(tok)
        if m[t] == nil { m[t] = make(map[string]struct{}) }
        m[t][scid] = struct{}{}
    }
    for _, app := range apps {
        scid, _ := app["scid"].(string)
        name, _ := app["name"].(string)
        descr, _ := app["description"].(string)
        durl, _ := app["durl"].(string)
        for _, w := range wordRE.FindAllString(durl, -1) { add(tokDURL, w, scid) }
        // Prefer display_name/name tokens
        disp, _ := app["display_name"].(string)
        if disp == "" { disp = name }
        for _, w := range wordRE.FindAllString(disp, -1) { add(tokName, w, scid) }
        for _, w := range wordRE.FindAllString(descr, -1) { add(tokDesc, w, scid) }
    }
    persistInvertedIndex(a, tokDURL, tokName, tokDesc, current)
}

// SearchTextIndex searches for tokens and returns ranked SCIDs
func (a *App) SearchTextIndex(query string) []string {
    if a.cache == nil || strings.TrimSpace(query) == "" { return nil }
    toks := wordRE.FindAllString(strings.ToLower(query), -1)
    if len(toks) == 0 { return nil }
    // weights: dURL 5, name 3, descr 1
    wD, wN, wX := 5, 3, 1
    score := make(map[string]int)
    for _, t := range toks {
        for _, sc := range loadInvertedPostingKind(a, t, 'd') { score[sc] += wD }
        for _, sc := range loadInvertedPostingKind(a, t, 'n') { score[sc] += wN }
        for _, sc := range loadInvertedPostingKind(a, t, 'x') { score[sc] += wX }
    }
    // rank
    type pair struct{ sc string; s int }
    arr := make([]pair, 0, len(score))
    for sc, s := range score { arr = append(arr, pair{sc, s}) }
    sort.Slice(arr, func(i, j int) bool {
        if arr[i].s != arr[j].s { return arr[i].s > arr[j].s }
        return arr[i].sc < arr[j].sc
    })
    out := make([]string, 0, len(arr))
    for _, p := range arr { out = append(out, p.sc) }
    return out
}

// helpers for numeric parsing if needed later
func atoi64(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }

