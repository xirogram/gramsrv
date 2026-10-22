package help

import (
	"context"
	"encoding/json"
	"testing"
)

// Production ships these four wallet keys; the test DC ships wallet_available
// false. The limits are denominated in nanoton, which is the unit telesrv's own
// TON ledger keeps balances in, so they are carried through unconverted.
func walletConfigValues(t *testing.T, s *Service) map[string]any {
	t.Helper()
	cfg, notModified, err := s.GetAppConfig(context.Background(), 0, 0)
	if err != nil || notModified {
		t.Fatalf("GetAppConfig = notModified %v err %v", notModified, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(cfg.JSON, &decoded); err != nil {
		t.Fatalf("app config json invalid: %v", err)
	}
	return decoded
}

// wallet_available is the gate a client checks before it renders the wallet
// entry point at all. An absent key and a false one both hide it, so the key has
// to be present either way.
func TestAppConfigWalletKeysFollowAvailability(t *testing.T) {
	on := walletConfigValues(t, NewService(nil, nil, WithWalletAvailable(true)))
	if available, ok := on["wallet_available"].(bool); !ok || !available {
		t.Fatalf("wallet_available = %v, want true when the wallet came up", on["wallet_available"])
	}

	off := walletConfigValues(t, NewService(nil, nil, WithWalletAvailable(false)))
	available, ok := off["wallet_available"].(bool)
	if !ok {
		t.Fatalf("wallet_available missing when disabled: %#v", off["wallet_available"])
	}
	if available {
		t.Fatal("wallet_available = true while the wallet is disabled: the client would offer an entry point that cannot work")
	}

	// No transfer or relay implementation exists in this server.
	for _, cfg := range []map[string]any{on, off} {
		if cfg["wallet_gasless_daily_transfers"] != float64(0) {
			t.Fatal("gasless must remain disabled")
		}
		for _, key := range []string{"wallet_transfer_min_nanos", "wallet_gasless_min_nanos", "ton_topup_url", "starref_connect_allowed"} {
			if _, ok := cfg[key]; ok {
				t.Fatalf("unsupported capability %s advertised", key)
			}
		}
	}
}

// The hash has to move with the flag. A client holding the previous hash is
// answered with appConfigNotModified and never sees the keys at all, so a config
// that changes without a matching hash change is indistinguishable from one that
// never changed.
func TestAppConfigHashVariesWithWalletAvailability(t *testing.T) {
	on, _, err := NewService(nil, nil, WithWalletAvailable(true)).GetAppConfig(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	off, _, err := NewService(nil, nil, WithWalletAvailable(false)).GetAppConfig(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if on.Hash == off.Hash {
		t.Fatalf("hash %d is identical with the wallet on and off: clients would keep the cached config", on.Hash)
	}

	// A client holding the old hash must be refreshed, not told notModified.
	fresh, notModified, err := NewService(nil, nil, WithWalletAvailable(true)).GetAppConfig(context.Background(), 0, off.Hash)
	if err != nil || notModified {
		t.Fatalf("client holding the previous hash got notModified=%v err=%v, want a refreshed config", notModified, err)
	}
	if fresh.Hash != on.Hash {
		t.Fatalf("refreshed hash = %d, want %d", fresh.Hash, on.Hash)
	}
}
