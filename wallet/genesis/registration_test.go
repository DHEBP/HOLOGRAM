package genesis

// Tests for the registration miner. TestBurnedVector_DerivesAndRegistrationComplete
// pins a frozen historical mainnet registration mined years before the 28-bit
// target existed — it only exercises the old 24-bit (3-zero-byte) check and
// says nothing about the 4th-byte nibble clause. 28-bit correctness is pinned
// here by TestRegistrationPoW_28BitBoundary, which calls the real production
// function (MeetsRegistrationTarget, shared by mineWorker and wallet.go's
// hot-wallet miner), plus the -short-gated live mine below (a genuine
// multi-minute 28-bit grind) which exercises the full mineWorker path. The
// rest of this file tests the logic THIS port adds around the algorithm:
// cancellation, the progress callback, and seed re-derivation — all fast.

import (
	"testing"
	"time"

	"github.com/deroproject/derohe/walletapi"
)

// ---- cancellation returns ("", nil) promptly and leaks no result ----
func TestMineRegistration_CancelReturnsPromptly(t *testing.T) {
	cleanGlobals(t)

	w, err := walletapi.Create_Encrypted_Wallet_Random_Memory("")
	if err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	w.SetOfflineMode()
	defer w.Close_Encrypted_Wallet()
	w.SetNetwork(true)

	cancel := make(chan struct{})
	done := make(chan struct{})
	var hexTx string
	var mineErr error
	go func() {
		hexTx, mineErr = MineRegistration(w, cancel, nil)
		close(done)
	}()

	// let it spin briefly, then cancel.
	time.Sleep(200 * time.Millisecond)
	close(cancel)

	select {
	case <-done:
		if mineErr != nil {
			t.Fatalf("cancelled mine should return nil error, got %v", mineErr)
		}
		if hexTx != "" {
			t.Fatalf("cancelled mine should return empty hex, got %d chars", len(hexTx))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MineRegistration did not return within 5s of cancel — workers not honoring cancel")
	}
}

// ---- the progress callback fires while mining ----
func TestMineRegistration_ProgressFires(t *testing.T) {
	cleanGlobals(t)

	w, err := walletapi.Create_Encrypted_Wallet_Random_Memory("")
	if err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	w.SetOfflineMode()
	defer w.Close_Encrypted_Wallet()
	w.SetNetwork(true)

	cancel := make(chan struct{})
	progressed := make(chan uint64, 1)
	go MineRegistration(w, cancel, func(attempts uint64) {
		select {
		case progressed <- attempts:
		default:
		}
	})

	// progress fires every 10k attempts; on any modern core this is well under 2s.
	select {
	case n := <-progressed:
		if n == 0 {
			t.Fatal("progress callback fired with 0 attempts")
		}
	case <-time.After(15 * time.Second):
		close(cancel)
		t.Fatal("no progress callback within 15s — counter or callback wiring is broken")
	}
	close(cancel)
}

// ---- seed re-derivation produces the burned wallet (MineRegistrationFromSeed's
// derivation half, without the multi-minute mine) ----
func TestMineRegistrationFromSeed_DerivesCorrectWallet(t *testing.T) {
	cleanGlobals(t)

	// Re-derive via the same path MineRegistrationFromSeed uses and confirm it
	// reconstructs the burned wallet's address — so a mined registration would
	// bind to the right key.
	account, err := walletapi.Generate_Account_From_Recovery_Words(testSeed)
	if err != nil {
		t.Fatalf("recover account: %v", err)
	}
	w, err := walletapi.Create_Encrypted_Wallet_Memory("", account.Keys.Secret)
	if err != nil {
		t.Fatalf("rebuild in-memory wallet: %v", err)
	}
	defer w.Close_Encrypted_Wallet()
	w.SetNetwork(true)
	if got := w.GetAddress().String(); got != testAddr {
		t.Fatalf("re-derived %s, want burned addr %s", got, testAddr)
	}
}

// ---- 28-bit target boundary: exercises the real production function
// (MeetsRegistrationTarget, called by both mineWorker and wallet.go's
// hot-wallet miner), not a local reimplementation — so a regression back to
// the old 24-bit-only check in EITHER call site fails this test. Pins the
// case that would have wrongly passed under that old (3-zero-byte-only)
// check. ----
func TestRegistrationPoW_28BitBoundary(t *testing.T) {
	cases := []struct {
		name string
		b3   byte // hash[3]
		want bool
	}{
		{"byte4 = 0x00 — meets 28-bit target", 0x00, true},
		{"byte4 = 0x0F (high nibble zero) — meets 28-bit target", 0x0F, true},
		{"byte4 = 0x10 (high nibble nonzero) — just misses 28-bit target", 0x10, false},
		{"byte4 = 0xF0 — meets old 24-bit target but fails 28-bit", 0xF0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var hash [32]byte
			hash[3] = c.b3
			if got := MeetsRegistrationTarget(hash); got != c.want {
				t.Fatalf("hash[3]=%#x: got %v, want %v", c.b3, got, c.want)
			}
		})
	}

	t.Run("byte3 nonzero — fails", func(t *testing.T) {
		var hash [32]byte
		hash[2] = 0x01
		if MeetsRegistrationTarget(hash) {
			t.Fatal("hash[2] nonzero must fail regardless of hash[3]")
		}
	})
}

// ---- full live mine: real 28-bit grind, -short-gated ----
func TestMineRegistrationFromSeed_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live 28-bit cold registration mine (mean ~16 min at 24-bit hashrate; longer at 28-bit); run without -short")
	}
	cleanGlobals(t)

	cancel := make(chan struct{})
	hexTx, err := MineRegistrationFromSeed(testSeed, NetworkMainnet, cancel, nil)
	if err != nil {
		t.Fatalf("live mine: %v", err)
	}
	if hexTx == "" {
		t.Fatal("live mine returned empty hex without cancel")
	}
	// MineRegistration already gates on IsRegistrationValid; a non-empty result
	// means it passed. Sanity-check it's the right length (101 bytes / 202 hex).
	if len(hexTx) != 202 {
		t.Fatalf("registration hex len = %d, want 202", len(hexTx))
	}
}
