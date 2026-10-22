package rpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"telesrv/internal/domain"
)

// dispatchWallet230 encodes a Layer 230 request, dispatches it, and returns the
// decoded result.
func dispatchWallet230(t *testing.T, r *Router, req bin.Object) any {
	t.Helper()
	ctx := WithAuthKeyID(WithUserID(context.Background(), 42), [8]byte{1})
	var body bin.Buffer
	if err := req.Encode(&body); err != nil {
		t.Fatal(err)
	}
	admitted, err := r.AdmitLayer(tlprofile.Profile230, &body, tlprofile.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	result, method, err := r.DispatchAdmitted(ctx, [8]byte{1}, 1, 0, 1, admitted)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	var out bin.Buffer
	if err = result.Encode(&out); err != nil {
		t.Fatal(err)
	}
	decoded, err := tlprofile.DecodeObject(tlprofile.Profile230, &out, tlprofile.Limits{})
	if err != nil {
		t.Fatal(method, err)
	}
	return decoded
}

// The methods below used to be unregistered, so every call logged an
// "Unhandled RPC (compatibility trace)" warning and the client got an
// unknown-method error. Registration is the fix; this test is what stops it
// regressing, because it asserts the warning is not emitted rather than only
// asserting the handler ran.
func TestWalletTonConnectAndCurrencyRatesDoNotTrace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		req     bin.Object
		wantErr string // "" means it must answer
	}{
		{"sessions", &tg.WalletTonConnectGetSessionsRequest{}, ""},
		{"nextEventID", &tg.WalletTonConnectNextEventIDRequest{}, ""},
		{"currencyRates", &tg.PaymentsGetCurrencyRatesRequest{}, ""},
		{"gasless", &tg.WalletGetGaslessInfoRequest{}, "WALLET_UNAVAILABLE"},
		{"createSession", &tg.WalletTonConnectCreateSessionRequest{}, "WALLET_UNAVAILABLE"},
		{"registerKey", &tg.WalletTonConnectRegisterKeyRequest{}, "WALLET_UNAVAILABLE"},
		{"getPending", &tg.WalletTonConnectGetPendingRequest{}, "WALLET_UNAVAILABLE"},
		{"submitConnectResult", &tg.WalletTonConnectSubmitConnectResultRequest{}, "WALLET_UNAVAILABLE"},
		{"claimRequest", &tg.WalletTonConnectClaimRequestRequest{}, "WALLET_UNAVAILABLE"},
		{"submitResponse", &tg.WalletTonConnectSubmitResponseRequest{}, "WALLET_UNAVAILABLE"},
		{"closeSession", &tg.WalletTonConnectCloseSessionRequest{}, "WALLET_UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.WarnLevel)
			r := New(Config{}, Deps{Wallet: &walletRPCFixture{}}, zap.New(core), clock.System)
			ctx := WithAuthKeyID(WithUserID(context.Background(), 42), [8]byte{1})

			var body bin.Buffer
			if err := tc.req.Encode(&body); err != nil {
				t.Fatal(err)
			}
			admitted, err := r.AdmitLayer(tlprofile.Profile230, &body, tlprofile.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			_, method, err := r.DispatchAdmitted(ctx, [8]byte{1}, 1, 0, 1, admitted)

			// The whole point: no compatibility trace for a method we register.
			for _, entry := range logs.All() {
				if strings.Contains(entry.Message, "Unhandled RPC") {
					t.Fatalf("%s still traced as unhandled: %v", method, entry.ContextMap())
				}
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("%s must answer, got %v", method, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("%s error = %v, want %s", method, err, tc.wantErr)
			}
			// The refusal has to be a client-handled capability error, not an
			// unknown-method error: the client is entitled to know the difference.
			var rpcErr *tgerr.Error
			if !errors.As(err, &rpcErr) || rpcErr.Code != 400 {
				t.Fatalf("%s error = %v, want a 400 tgerr", method, err)
			}
		})
	}
}

// The answering methods must return the honest empty answer, not nil: an empty
// session list is "there are none", which is true, where nil would be a decode
// failure the client cannot distinguish from an error.
func TestWalletTonConnectEmptyAnswersAreNotNil(t *testing.T) {
	r := New(Config{}, Deps{Wallet: &walletRPCFixture{}}, zap.NewNop(), clock.System)

	sessions := dispatchWallet230(t, r, &tg.WalletTonConnectGetSessionsRequest{})
	typed, ok := sessions.(*tg.WalletTonConnectSessions)
	if !ok {
		t.Fatalf("sessions = %T, want *tg.WalletTonConnectSessions", sessions)
	}
	// Length, not nil-ness: an empty vector encodes to zero bytes and decodes
	// back as a nil slice, so nil is what an honest empty list looks like on the
	// far side of the wire.
	if len(typed.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none", typed.Sessions)
	}

	// Currency rates are the exception to the "empty means none" rule. The client
	// calls this on every Money-screen open -- 434 times in the journal, more than
	// any other payments method -- and renders the fiat amount beside the balance
	// from the answer. An empty vector is not "no rates to quote", it is "this
	// server has no rates", and the client duly showed a real balance with no USD
	// next to it. So the rate is reported, from the same constant appConfig uses.
	rates := dispatchWallet230(t, r, &tg.PaymentsGetCurrencyRatesRequest{})
	typedRates, ok := rates.(*tg.PaymentsCurrencyRates)
	if !ok {
		t.Fatalf("rates = %T, want *tg.PaymentsCurrencyRates", rates)
	}
	if len(typedRates.Rates) != 1 {
		t.Fatalf("rates = %#v, want exactly one USD rate", typedRates.Rates)
	}
	if typedRates.Rates[0].Currency != "USD" || typedRates.Rates[0].Rate != domain.TonUSDRate {
		t.Fatalf("rate = %#v, want USD at %v", typedRates.Rates[0], domain.TonUSDRate)
	}
}

// A client that has a wallet must see a balance, and a brand new one has none.
// This is the contract behind "the wallet shows 0": the RPC carries the number
// through faithfully rather than omitting or inventing it.
func TestWalletStateCarriesZeroBalanceForANewWallet(t *testing.T) {
	f := &walletRPCFixture{}
	f.links = []domain.WalletLink{{UserID: 42, Address: "EQ-fresh"}}
	r := New(Config{}, Deps{Wallet: f}, zap.NewNop(), clock.System)
	ctx := WithAuthKeyID(WithUserID(context.Background(), 42), [8]byte{1})

	state, err := r.onWalletState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	typed, ok := state.(*tg.WalletState)
	if !ok {
		t.Fatalf("state = %T, want *tg.WalletState", state)
	}
	if typed.Address == "" {
		t.Fatal("a bound wallet must carry an address")
	}
	if typed.Balance != 0 {
		t.Fatalf("balance = %d, want 0", typed.Balance)
	}
}
