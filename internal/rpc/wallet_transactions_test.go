package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap"

	"telesrv/internal/domain"
)

// The Money screen renders no history at all until wallet.getTransactions
// answers with something, and an always-empty page is indistinguishable from a
// broken one. So the page is served from telesrv's own ledger, which is where
// the entries that move TON are actually written.
func TestWalletTransactionsServesLedgerHistory(t *testing.T) {
	f := &walletRPCFixture{
		balance: 4200,
		txns: []domain.TonTransaction{
			{ID: 11, Amount: -500, Date: 1700000000, Reason: "ton_suggested_post", Peer: domain.Peer{Type: domain.PeerTypeChannel, ID: 7}},
			{ID: 12, Amount: 1500, Date: 1700000100, Reason: "ton_stargift", Peer: domain.Peer{Type: domain.PeerTypeUser, ID: 42}},
		},
	}
	r := New(Config{}, Deps{Wallet: f}, zap.NewNop(), clock.System)
	ctx := WithAuthKeyID(WithUserID(context.Background(), 42), [8]byte{1})

	out, err := r.onWalletTransactions(ctx, &tg.WalletGetTransactionsRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	page, ok := out.(*tg.WalletTransactions)
	if !ok {
		t.Fatalf("result = %T, want *tg.WalletTransactions", out)
	}
	if page.Balance != 4200 {
		t.Fatalf("balance = %d, want 4200", page.Balance)
	}
	if len(page.Transactions) != 2 {
		t.Fatalf("transactions = %d, want 2", len(page.Transactions))
	}
	first := page.Transactions[0]
	if first.ID != "11" {
		t.Fatalf("first.ID = %q, want \"11\"", first.ID)
	}
	if first.Amount != -500 {
		t.Fatalf("first.Amount = %d, want -500", first.Amount)
	}
	if first.Date != 1700000000 {
		t.Fatalf("first.Date = %d, want 1700000000", first.Date)
	}
	if first.Incoming {
		t.Fatal("a negative amount must not be reported as incoming")
	}
	if _, ok := first.Peer.(*tg.WalletTransactionPeerUnsupported); !ok {
		t.Fatalf("a channel peer must not be reported as a user: %T", first.Peer)
	}
	second := page.Transactions[1]
	if !second.Incoming || second.Amount != 1500 {
		t.Fatalf("second transaction = %+v", second)
	}
	if peer, ok := second.Peer.(*tg.WalletTransactionPeerUser); !ok || peer.UserID != 42 {
		t.Fatalf("user peer = %T", second.Peer)
	}
}

// A deployment with no ledger history must render an empty page, not an error:
// the client shows an empty state for that, and failing the read would replace it
// with an error instead.
func TestWalletTransactionsEmptyIsNotAnError(t *testing.T) {
	f := &walletRPCFixture{balance: 0}
	r := New(Config{}, Deps{Wallet: f}, zap.NewNop(), clock.System)
	ctx := WithAuthKeyID(WithUserID(context.Background(), 42), [8]byte{1})

	out, err := r.onWalletTransactions(ctx, &tg.WalletGetTransactionsRequest{})
	if err != nil {
		t.Fatalf("empty history must not fail: %v", err)
	}
	page := out.(*tg.WalletTransactions)
	if page.Transactions == nil {
		t.Fatal("transactions must be an empty list, not nil")
	}
	if len(page.Transactions) != 0 {
		t.Fatalf("transactions = %d, want 0", len(page.Transactions))
	}
}
