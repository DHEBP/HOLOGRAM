package main

import (
	"testing"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
)

func TestParseXSWDScArgs_JunkWithoutEntrypointReturnsEmpty(t *testing.T) {
	params := map[string]interface{}{
		"sc_rpc": []interface{}{
			map[string]interface{}{"name": "dummy", "datatype": "S", "value": "x"},
		},
	}
	args := parseXSWDScArgs(params, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if len(args) != 0 {
		t.Fatalf("junk sc_rpc without entrypoint must yield empty args, got %d: %#v", len(args), args)
	}
	if scArgsAreRealCall(args) {
		t.Fatal("empty args must not count as a real SC call")
	}
}

func TestParseXSWDScArgs_EntrypointIsRealCall(t *testing.T) {
	scid := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	params := map[string]interface{}{
		"entrypoint": "Deposit",
		"sc_rpc": []interface{}{
			map[string]interface{}{"name": "amount", "datatype": "U", "value": float64(1)},
		},
	}
	args := parseXSWDScArgs(params, scid)
	if !scArgsAreRealCall(args) {
		t.Fatalf("entrypoint+SCACTION expected, got %#v", args)
	}
}

func TestScArgsAreRealCall_RequiresBoth(t *testing.T) {
	if scArgsAreRealCall(nil) {
		t.Fatal("nil must be false")
	}
	onlyAction := rpc.Arguments{
		{Name: rpc.SCACTION, DataType: "U", Value: uint64(rpc.SC_CALL)},
	}
	if scArgsAreRealCall(onlyAction) {
		t.Fatal("SCACTION alone is not a real call")
	}
	onlyEP := rpc.Arguments{
		{Name: "entrypoint", DataType: "S", Value: "Foo"},
	}
	if scArgsAreRealCall(onlyEP) {
		t.Fatal("entrypoint alone is not a real call")
	}
	both := rpc.Arguments{
		{Name: rpc.SCACTION, DataType: "U", Value: uint64(rpc.SC_CALL)},
		{Name: "entrypoint", DataType: "S", Value: "Foo"},
	}
	if !scArgsAreRealCall(both) {
		t.Fatal("SCACTION+entrypoint must be a real call")
	}
}

func TestJunkScRPCCannotBypassBurnGuard(t *testing.T) {
	// The attack shape from R2-B6: junk sc_rpc + native burn + destination.
	params := map[string]interface{}{
		"scid": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"sc_rpc": []interface{}{
			map[string]interface{}{"name": "dummy", "datatype": "S", "value": "x"},
		},
	}
	scArgs := parseXSWDScArgs(params, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	transfers := []rpc.Transfer{{
		Destination: "dero1qytest",
		Amount:      0,
		Burn:        1500000000,
		SCID:        crypto.ZEROHASH,
	}}
	burnAmt, block := shouldBlockBurn(transfers, scArgsAreRealCall(scArgs))
	if !block {
		t.Fatal("junk sc_rpc must NOT count as SC call — native burn must be blocked")
	}
	if burnAmt != 1500000000 {
		t.Fatalf("burn amount = %d, want 1500000000", burnAmt)
	}
}
