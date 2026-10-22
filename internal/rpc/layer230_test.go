package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLayer230AdditionalBotUsernameRejected(t *testing.T) {
	r, _, _, owner, _ := newBotRPCTestRouter(t)
	ctx := WithUserID(context.Background(), owner.ID)
	for _, raw := range []bool{false, true} {
		req := &tg.BotsCheckUsernameRequest{Username: "fresh_rpc_bot"}
		if raw {
			req.Flags.Set(0)
		} else {
			req.Additional = true
		}
		if ok, err := r.onBotsCheckUsername(ctx, req); ok || !tgerr.Is(err, "METHOD_INVALID") {
			t.Fatalf("additional=%v: %v, %v", raw, ok, err)
		}
	}
	if ok, err := r.onBotsCheckUsername(ctx, &tg.BotsCheckUsernameRequest{Username: "fresh_rpc_bot"}); !ok || err != nil {
		t.Fatalf("ordinary username: %v, %v", ok, err)
	}
}

// Registered read stubs and unsupported capabilities must have explicit results.
// Client polling does not make an unimplemented transfer/relay successful.
func TestLayer230WalletSurfaceIsFullyRegistered(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	// A wallet service is present on purpose: the point is what each method
	// answers when the wallet exists. A router without one reports
	// WALLET_UNAVAILABLE for all of them, which is covered separately.
	r := New(Config{DC: 2, IP: "127.0.0.1", Port: 2398}, Deps{Wallet: &walletRPCFixture{}}, zap.New(core), clock.System)

	ctx := WithAuthKeyID(WithUserID(context.Background(), 1), [8]byte{1})
	answer := map[string]bin.Object{
		"getNfts":      &tg.WalletGetNftsRequest{},
		"tonConnect":   &tg.WalletTonConnectGetSessionsRequest{},
		"onramp":       &tg.PaymentsGetOnrampProvidersRequest{},
		"transactions": &tg.WalletGetTransactionsRequest{},
	}
	for name, query := range answer {
		wire := bin.Buffer{}
		if err := (&tg.InvokeWithLayerRequest{Layer: 230, Query: query}).Encode(&wire); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.DispatchWithMethod(ctx, [8]byte{1}, 230, &wire); err != nil {
			t.Fatalf("%s must answer, got %v", name, err)
		}
	}

	for name, query := range map[string]bin.Object{
		"gasless":          &tg.WalletGetGaslessInfoRequest{},
		"streaming":        &tg.ToncenterGetStreamingURLRequest{},
		"tonConnectCreate": &tg.WalletTonConnectCreateSessionRequest{},
	} {
		wire := bin.Buffer{}
		if err := (&tg.InvokeWithLayerRequest{Layer: 230, Query: query}).Encode(&wire); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.DispatchWithMethod(ctx, [8]byte{1}, 230, &wire); !tgerr.Is(err, "WALLET_UNAVAILABLE") {
			t.Fatalf("%s: %v, want WALLET_UNAVAILABLE", name, err)
		}
	}

	// Nothing on the wallet surface may trace: a traced method is a warning per
	// poll, and these are polled continuously.
	if got := logs.FilterMessage("Unhandled RPC (compatibility trace)").Len(); got != 0 {
		t.Fatalf("trace count = %d, want 0: %+v", got, logs.All())
	}
}
