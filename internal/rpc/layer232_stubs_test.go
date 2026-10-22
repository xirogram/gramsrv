package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"
	"go.uber.org/zap/zaptest"
)

// TestLayer232StubsRefuseWithNotImplemented pins the Layer 232 methods that
// telesrv knows but does not implement: the wire schema exists, so the client
// gets an explicit NOT_IMPLEMENTED instead of an unknown-method error.
func TestLayer232StubsRefuseWithNotImplemented(t *testing.T) {
	r := New(Config{}, Deps{}, zaptest.NewLogger(t), clock.System)
	ctx := WithUserID(context.Background(), 1000000001)
	for _, tc := range []struct {
		name    string
		request bin.Object
	}{
		{name: "messages.setShareTheme", request: &tg.MessagesSetShareThemeRequest{
			Peer: &tg.InputPeerSelf{}, Theme: &tg.ShareThemeDefault{},
		}},
		{name: "messages.getShareThemeBackdrops", request: &tg.MessagesGetShareThemeBackdropsRequest{Hash: 0}},
		{name: "payments.withdrawStarGift", request: &tg.PaymentsWithdrawStarGiftRequest{
			Stargift: &tg.InputSavedStarGiftUser{MsgID: 1},
		}},
		{name: "payments.depositStarGift", request: &tg.PaymentsDepositStarGiftRequest{
			Peer: &tg.InputPeerSelf{}, Slug: "stub",
		}},
		{name: "payments.displayStarGift", request: &tg.PaymentsDisplayStarGiftRequest{Slug: "stub"}},
		{name: "contacts.exportNoPaidMessagesToken", request: &tg.ContactsExportNoPaidMessagesTokenRequest{}},
		{name: "contacts.importNoPaidMessagesToken", request: &tg.ContactsImportNoPaidMessagesTokenRequest{
			UserID: &tg.InputUserSelf{}, Token: "stub",
		}},
		{name: "wallet.setCardStickers", request: &tg.WalletSetCardStickersRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := encodeExactLayerRPC(t, tlprofile.Profile232, tc.request)
			admitted, err := r.AdmitLayer(tlprofile.Profile232, &body, tlprofile.Limits{})
			if err != nil {
				t.Fatalf("admit exact Layer 232 request: %v", err)
			}
			_, method, err := r.DispatchAdmitted(ctx, [8]byte{}, 0, 0, 0, admitted)
			if err == nil {
				t.Fatalf("%s returned no error (method %q)", tc.name, method)
			}
			if !tgerr.Is(err, "NOT_IMPLEMENTED") {
				t.Fatalf("%s err = %v, want NOT_IMPLEMENTED", tc.name, err)
			}
		})
	}
}
