package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The XSWD plane must hand a dApp exactly what derod sent. derod hex-encodes every SC string
// variable on its way out (cmd/derod/rpc/rpc_dero_getsc.go), so hex is the wire format every
// dApp — including ones written against Engram — already decodes for itself. HOLOGRAM decoding
// first made the same app show different data depending on which wallet ran it, and there is no
// downstream test that tells the two apart: "CAFE" is valid hex charset either way.
//
// routeDaemonCall has two callers, the WebSocket server (xswd_server.go) and the in-app bridge
// (app.go), so this pins both planes. Explorer's own DaemonGetSC still decodes — that is display,
// not protocol, and is deliberately not covered here.
//
// Prove this test has teeth by restoring the normalize call in routeDaemonCall: it must fail.
func TestRouteDaemonCall_GetSCIsRawPassthrough(t *testing.T) {
	// hex("lolz") — printable ASCII once decoded, so the old normalizer would have eaten it.
	const hexName = "6c6f6c7a"
	// hex("deto1qy…"), the shape that made a creator address render decoded inside HOLOGRAM
	// and raw everywhere else.
	const hexAddr = "6465746f31717977737866347a"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]interface{}{
				"status": "OK",
				"stringkeys": map[string]interface{}{
					"name":    hexName,
					"creator": hexAddr,
				},
			},
		})
	}))
	defer server.Close()

	app := &App{daemonClient: NewDaemonClient(server.URL)}

	resp := app.routeDaemonCall("DERO.GetSC", map[string]interface{}{"scid": "deadbeef"})

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("no result in response: %#v", resp)
	}
	keys, ok := result["stringkeys"].(map[string]interface{})
	if !ok {
		t.Fatalf("no stringkeys in result: %#v", result)
	}

	if got := keys["name"]; got != hexName {
		t.Errorf("name = %v, want the raw hex %q — the XSWD plane decoded it", got, hexName)
	}
	if got := keys["creator"]; got != hexAddr {
		t.Errorf("creator = %v, want the raw hex %q — the XSWD plane decoded it", got, hexAddr)
	}
}
