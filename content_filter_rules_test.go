// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Regression tests for evaluateRule's author and category rules (issue #27).

package main

import "testing"

// Two renderings of ONE key. A contract stores the dero1 form on every network,
// because ADDRESS_STRING builds through rpc.NewAddressFromKeys and leaves Mainnet
// true; consensus cannot depend on which network a node believes it is on. A wallet
// opened on a simulator shows the deto1 form of the same key. Addresses are bech32,
// so the prefix names the network and the checksum is computed over it: both halves
// of the string differ.
const (
	authorMainnet   = "dero1qyw4fl3dupcg5qlrcsvcedze507q9u67lxfpu8kgnzp04aq73yheqqg2ctjn4"
	authorSimulator = "deto1qyw4fl3dupcg5qlrcsvcedze507q9u67lxfpu8kgnzp04aq73yheqqgsph2ka"
	otherAuthor     = "dero1qy976ssakhfynpd4lnh39u7gw9spfzr9z55ckfd0yhrhsdr235glgqq28xlvm"
)

func ruleFor(t, op, v string) FilterRule {
	return FilterRule{Type: t, Operator: op, Value: v, Enabled: true}
}

// TestAuthorRuleEqMatchesAcrossNetworkRendering is the half of issue #27 that made an
// author rule unusable the moment anyone could write one: the rule value is whatever
// the user copied out of their wallet, and the app's author is whatever the contract
// stored, so on a simulator the two are the same key spelled two ways.
func TestAuthorRuleEqMatchesAcrossNetworkRendering(t *testing.T) {
	if authorMainnet == authorSimulator {
		t.Fatal("the two vectors are identical; this test proves nothing")
	}

	cases := []struct {
		name      string
		appAuthor string
		ruleValue string
		wantMatch bool
	}{
		{"same rendering", authorMainnet, authorMainnet, true},
		{"app mainnet, rule simulator", authorMainnet, authorSimulator, true},
		{"app simulator, rule mainnet", authorSimulator, authorMainnet, true},
		{"different author", authorMainnet, otherAuthor, false},
		{"different author across networks", authorSimulator, otherAuthor, false},
		{"rule is not an address", authorMainnet, "nobody", false},
		{"app author empty", "", authorMainnet, false},
	}

	f := &ContentFilter{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := AppInfo{Author: tc.appAuthor}
			got := f.evaluateRule(ruleFor("author", "eq", tc.ruleValue), app, 0)
			if got != tc.wantMatch {
				t.Fatalf("author eq %q vs %q = %v, want %v",
					tc.appAuthor, tc.ruleValue, got, tc.wantMatch)
			}
		})
	}
}

// TestContainsOperatorIsSubstring is the other half of issue #27: both contains arms
// were a copy of their eq arm, so "contains" behaved as exact equality and a rule
// using it could never match a longer string.
func TestContainsOperatorIsSubstring(t *testing.T) {
	f := &ContentFilter{}

	cases := []struct {
		name      string
		ruleType  string
		app       AppInfo
		value     string
		wantMatch bool
	}{
		{"category substring", "category", AppInfo{Category: "games/arcade"}, "arcade", true},
		{"category exact still matches", "category", AppInfo{Category: "games"}, "games", true},
		{"category absent", "category", AppInfo{Category: "games"}, "finance", false},
		{"author substring", "author", AppInfo{Author: authorMainnet}, "qyw4fl3", true},
		{"author absent", "author", AppInfo{Author: authorMainnet}, "zzzzzz", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := f.evaluateRule(ruleFor(tc.ruleType, "contains", tc.value), tc.app, 0)
			if got != tc.wantMatch {
				t.Fatalf("%s contains %q = %v, want %v", tc.ruleType, tc.value, got, tc.wantMatch)
			}
		})
	}

	// Non-vacuity: the substring cases above must be ones an equality compare would
	// REJECT, or this test would pass against the defect it exists to catch.
	if "games/arcade" == "arcade" || authorMainnet == "qyw4fl3" {
		t.Fatal("the substring vectors are exact matches; this test cannot see the defect")
	}
}

// TestUnknownRuleShapesStayClosed pins the surrounding behaviour the fix must not
// widen: an unknown type or operator matches nothing.
func TestUnknownRuleShapesStayClosed(t *testing.T) {
	f := &ContentFilter{}
	app := AppInfo{Author: authorMainnet, Category: "games"}

	if f.evaluateRule(ruleFor("keyword", "eq", "games"), app, 0) {
		t.Fatal("an unimplemented rule type must not match")
	}
	if f.evaluateRule(ruleFor("author", "not_contains", authorMainnet), app, 0) {
		t.Fatal("an unimplemented operator must not match")
	}
}
