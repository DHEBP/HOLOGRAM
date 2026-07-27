package main

import "testing"

// The on-chain TELA contract declares `Function Rate(r Uint64)`. The DVM binds
// arguments by the contract's declared parameter name, so the sc_rpc arg MUST be
// named "r". Passing any other name (e.g. "rating") fails with
// `Argument "r" is missing while invoking "Rate"` and the rating never records.
func TestBuildRateArgs_UsesContractParamName(t *testing.T) {
	args := buildRateArgs(88)

	if len(args) != 1 {
		t.Fatalf("expected exactly 1 arg, got %d", len(args))
	}

	if name := args[0]["name"]; name != "r" {
		t.Fatalf(`Rate arg name must be "r" to match the on-chain contract, got %q`, name)
	}
	if dt := args[0]["datatype"]; dt != "U" {
		t.Fatalf(`Rate arg datatype must be "U" (Uint64), got %q`, dt)
	}
	if v := args[0]["value"]; v != uint64(88) {
		t.Fatalf("Rate arg value must be uint64(88), got %#v", v)
	}
}
