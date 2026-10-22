package help

import (
	"encoding/json"
	"testing"

	"telesrv/internal/domain"
)

// The app config is assembled by string concatenation, so a fragment missing a
// comma does not fail the build -- it produces JSON the client cannot parse,
// and the client then falls back to its built-in defaults. That failure is
// invisible from the server: every request still succeeds and the wallet still
// opens, just without the limits, gasless copy or USD rate we meant to send.
//
// So the served bytes get parsed here, and the wallet keys are pinned to the
// values production ships. A USD amount that never renders is otherwise
// indistinguishable from a rate the client decided to ignore.
func TestDefaultAppConfigJSONParsesAndCarriesWalletKeys(t *testing.T) {
	raw := defaultAppConfigJSONWithPremiumBot("", "", true)
	if !json.Valid(raw) {
		t.Fatal("served app config is not valid JSON; the client will discard it")
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"ton_usd_rate":                   domain.TonUSDRate,
		"wallet_available":               true,
		"wallet_gasless_daily_transfers": float64(0),
	} {
		if got := cfg[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

// wallet_available is the switch that makes the client show the Money tab at
// all, and the JSON is only valid if the fragment lands before the closing
// brace. Turning it off must produce the same document minus that one key.
func TestDefaultAppConfigWithoutWalletStaysValid(t *testing.T) {
	raw := defaultAppConfigJSONWithPremiumBot("", "", false)
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("app config without wallet keys is not valid JSON: %v", err)
	}
	if got := cfg["wallet_available"]; got != false {
		t.Fatalf("wallet_available = %#v, want false", got)
	}
	if _, ok := cfg["ton_usd_rate"]; !ok {
		t.Fatal("ton_usd_rate must survive with the wallet unavailable")
	}
}
