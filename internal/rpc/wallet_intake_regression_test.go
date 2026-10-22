package rpc

import (
	"context"
	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"
	"go.uber.org/zap"
	"telesrv/internal/domain"
	"testing"
)

type intakeWalletPage struct {
	walletRPCFixture
	query domain.StarsTransactionQuery
}

func (f *intakeWalletPage) Transactions(_ context.Context, _ int64, q domain.StarsTransactionQuery) (domain.TonTransactionPage, error) {
	f.query = q
	return domain.TonTransactionPage{Balance: 900, NextOffset: "123", Transactions: []domain.TonTransaction{{ID: 124, Amount: 5}}}, nil
}
func TestWalletExistingBalanceRequiresAuthorization(t *testing.T) {
	r := New(Config{}, Deps{}, zap.NewNop(), clock.System)
	var body bin.Buffer
	if err := (&tg.WalletGetExistingWaltBalanceRequest{}).Encode(&body); err != nil {
		t.Fatal(err)
	}
	admitted, err := r.AdmitLayer(tlprofile.Profile230, &body, tlprofile.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.DispatchAdmitted(context.Background(), [8]byte{1}, 1, 0, 1, admitted)
	if !tgerr.Is(err, "AUTH_KEY_UNREGISTERED") {
		t.Fatalf("unauthorized existing balance = %v", err)
	}
}
func TestWalletTransactionsPreservesCursorAndWireFlag(t *testing.T) {
	f := &intakeWalletPage{}
	r := New(Config{}, Deps{Wallet: f}, zap.NewNop(), clock.System)
	out := dispatchWallet230(t, r, &tg.WalletGetTransactionsRequest{Limit: 1}).(*tg.WalletTransactions)
	if offset, ok := out.GetNextOffset(); !ok || offset != "123" {
		t.Fatalf("next cursor = %q, present=%v", offset, ok)
	}
	if len(out.Transactions) != 1 || !out.Transactions[0].Incoming {
		t.Fatalf("wire transaction = %+v", out.Transactions)
	}
}
func TestWalletProvisionFailureIsNotAnEmptyWallet(t *testing.T) {
	f := &walletRPCFixture{createErr: domain.ErrWalletProofInvalid}
	r := New(Config{}, Deps{Wallet: f}, zap.NewNop(), clock.System)
	ctx := WithAuthKeyID(WithUserID(context.Background(), 42), [8]byte{1})
	_, err := r.onWalletState(ctx)
	if err == nil {
		t.Fatal("failed creation was silently returned as an empty wallet")
	}
}

func TestWalletTonConnectNextEventResultWire(t *testing.T) {
	r := New(Config{}, Deps{Wallet: &walletRPCFixture{}}, zap.NewNop(), clock.System)
	got := dispatchWallet230(t, r, &tg.WalletTonConnectNextEventIDRequest{})
	event, ok := got.(*tg.TonConnectNextEventID)
	if !ok || event.EventID != 0 {
		t.Fatalf("event response = %T %+v", got, got)
	}
}
