package main

import (
	"encoding/json"
	"testing"
)

// derod refuses a params object on its no-argument methods: DERO.GetHeight answers
// -32602 "no parameters accepted" to {"params":{}} and succeeds when the key is absent.
// Found by the consent probe, which sends {} like any ordinary JSON-RPC client.
func TestDaemonParams_EmptyBecomesAbsent(t *testing.T) {
	if got := daemonParams(nil); got != nil {
		t.Fatalf("nil map must stay absent, got %#v", got)
	}
	if got := daemonParams(map[string]interface{}{}); got != nil {
		t.Fatalf("empty map must become absent, got %#v", got)
	}

	real := map[string]interface{}{"scid": "deadbeef"}
	if got := daemonParams(real); got == nil {
		t.Fatal("a populated map must be forwarded")
	}
}

// The whole point is what lands on the wire. An interface holding a nil map is NOT nil, so
// `omitempty` does not fire for it — this pins that the request really omits the key.
func TestDaemonParams_OmitsTheKeyOnTheWire(t *testing.T) {
	encode := func(params map[string]interface{}) string {
		req := RPCRequest{
			JSONRPC: "2.0",
			ID:      "1",
			Method:  "DERO.GetHeight",
			Params:  daemonParams(params),
		}
		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(b)
	}

	if body := encode(map[string]interface{}{}); jsonHasKey(t, body, "params") {
		t.Fatalf(`empty params must not reach the daemon, got %s`, body)
	}
	if body := encode(map[string]interface{}{"scid": "x"}); !jsonHasKey(t, body, "params") {
		t.Fatalf("real params must survive, got %s", body)
	}
}

func jsonHasKey(t *testing.T, body, key string) bool {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
	_, ok := m[key]
	return ok
}
