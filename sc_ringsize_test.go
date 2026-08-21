package main

import "testing"

// A nameservice registration/transfer must sign with a real key (ring 2). A ring>2 call
// yields an all-zero SIGNER(), so the name lands owned by nobody and anyone can seize it.
// This pins that the nameservice SCID stays ring 2 even when the caller asks for anonymity.
func TestScinvokeRingsize_NameserviceStaysRing2(t *testing.T) {
	const otherSCID = "1111111111111111111111111111111111111111111111111111111111111111"

	cases := []struct {
		name      string
		scid      string
		anonymous bool
		want      uint64
	}{
		{"ordinary SC, attributable", otherSCID, false, 2},
		{"ordinary SC, anonymous", otherSCID, true, 16},
		{"nameservice, attributable", nameServiceSCID, false, 2},
		{"nameservice, anonymous stays ring 2", nameServiceSCID, true, 2},
	}

	for _, c := range cases {
		if got := scinvokeRingsize(c.scid, c.anonymous); got != c.want {
			t.Errorf("%s: scinvokeRingsize(%q, %v) = %d, want %d", c.name, c.scid, c.anonymous, got, c.want)
		}
	}
}
