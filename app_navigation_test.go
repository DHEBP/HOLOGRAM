package main

import "testing"

// TestPickServableSCID covers which deployment of a dURL gets opened.
//
// The real case: telatomicswaps.tela resolves to 13 SCIDs. Gnomon ranks a V5.3
// build first, but that build does not match TELA-INDEX-1, so the browser drops
// to srcdoc with limited wallet access - while the publisher's conforming V5.6
// sits one entry below under the same name.
func TestPickServableSCID(t *testing.T) {
	servableSet := func(ok ...string) func(string) bool {
		set := map[string]bool{}
		for _, s := range ok {
			set[s] = true
		}
		return func(c string) bool { return set[c] }
	}

	tests := []struct {
		name        string
		candidates  []string
		servable    func(string) bool
		wantSCID    string
		wantSkipped int
		wantOK      bool
	}{
		{
			name:        "top candidate is servable, ranking untouched",
			candidates:  []string{"a", "b", "c"},
			servable:    servableSet("a", "b"),
			wantSCID:    "a",
			wantSkipped: 0,
			wantOK:      true,
		},
		{
			name:        "skips an unservable top candidate",
			candidates:  []string{"v5.3", "v5.6"},
			servable:    servableSet("v5.6"),
			wantSCID:    "v5.6",
			wantSkipped: 1,
			wantOK:      true,
		},
		{
			// Never worse than before: with nothing servable, keep the old pick
			// rather than refusing to navigate.
			name:        "none servable falls back to the top candidate",
			candidates:  []string{"a", "b"},
			servable:    servableSet(),
			wantSCID:    "a",
			wantSkipped: 0,
			wantOK:      true,
		},
		{
			name:       "no candidates",
			candidates: nil,
			servable:   servableSet("a"),
			wantSCID:   "",
			wantOK:     false,
		},
		{
			name:        "single unservable candidate is still returned",
			candidates:  []string{"only"},
			servable:    servableSet(),
			wantSCID:    "only",
			wantSkipped: 0,
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scid, skipped, ok := pickServableSCID(tt.candidates, tt.servable)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if scid != tt.wantSCID {
				t.Errorf("scid = %q, want %q", scid, tt.wantSCID)
			}
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", skipped, tt.wantSkipped)
			}
		})
	}
}

// TestPickServableSCID_StopsAtFirstMatch pins the cost claim: the walk must not
// keep validating after it has found a servable contract. Validation is a
// daemon round trip on the navigation path.
func TestPickServableSCID_StopsAtFirstMatch(t *testing.T) {
	calls := 0
	scid, _, ok := pickServableSCID([]string{"a", "b", "c"}, func(string) bool {
		calls++
		return true
	})
	if !ok || scid != "a" {
		t.Fatalf("got %q ok=%v, want \"a\" ok=true", scid, ok)
	}
	if calls != 1 {
		t.Errorf("validated %d candidates, want 1", calls)
	}
}
