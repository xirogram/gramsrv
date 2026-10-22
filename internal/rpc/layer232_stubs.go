package rpc

import (
	"context"

	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tlprofile"
)

// Layer 232 added these methods. The wire schema is generated, but the
// business behind them is not implemented in telesrv yet: a client that calls
// one of them gets an explicit NOT_IMPLEMENTED instead of an unknown-method
// error, so the surface is visible and the refusal is honest.
//
// Each stub owns its own semantic registration, so a future implementation
// replaces exactly one handler here.

// onMessagesSetShareTheme stores a share theme on a peer profile.
func (r *Router) onMessagesSetShareTheme(ctx context.Context, req *tg.MessagesSetShareThemeRequest) (bool, error) {
	return false, notImplementedErr()
}

// onMessagesGetShareThemeBackdrops returns the share theme backdrop gallery.
func (r *Router) onMessagesGetShareThemeBackdrops(ctx context.Context, req *tg.MessagesGetShareThemeBackdropsRequest) (*tg.ShareThemeBackdrops, error) {
	return nil, notImplementedErr()
}

// onPaymentsWithdrawStarGift withdraws a collectible gift to the wallet.
func (r *Router) onPaymentsWithdrawStarGift(ctx context.Context, req *tg.PaymentsWithdrawStarGiftRequest) (bool, error) {
	return false, notImplementedErr()
}

// onPaymentsDepositStarGift deposits a collectible gift to a peer.
func (r *Router) onPaymentsDepositStarGift(ctx context.Context, req *tg.PaymentsDepositStarGiftRequest) (*tg.PaymentsStarGiftDepositPayload, error) {
	return nil, notImplementedErr()
}

// onPaymentsDisplayStarGift shows a collectible gift on a peer profile.
func (r *Router) onPaymentsDisplayStarGift(ctx context.Context, req *tg.PaymentsDisplayStarGiftRequest) (bool, error) {
	return false, notImplementedErr()
}

// onContactsExportNoPaidMessagesToken exports a no-paid-messages token.
func (r *Router) onContactsExportNoPaidMessagesToken(ctx context.Context, req *tg.ContactsExportNoPaidMessagesTokenRequest) (*tg.ExportedNoPaidMessagesToken, error) {
	return nil, notImplementedErr()
}

// onContactsImportNoPaidMessagesToken imports a no-paid-messages token.
func (r *Router) onContactsImportNoPaidMessagesToken(ctx context.Context, req *tg.ContactsImportNoPaidMessagesTokenRequest) (*tg.User, error) {
	return nil, notImplementedErr()
}

// onWalletSetCardStickers sets the wallet card stickers.
func (r *Router) onWalletSetCardStickers(ctx context.Context, req *tg.WalletSetCardStickersRequest) (*tg.WalletCard, error) {
	return nil, notImplementedErr()
}

// registerLayer232Stubs registers the Layer 232 methods that are known but not
// implemented. Every stub answers NOT_IMPLEMENTED.
func (r *Router) registerLayer232Stubs(d *tlprofile.Dispatcher) {
	registerRPC[*tg.MessagesSetShareThemeRequest](d, tlprofile.SemanticMethodMessagesSetShareTheme, func(ctx context.Context, req *tg.MessagesSetShareThemeRequest) (any, error) {
		return r.onMessagesSetShareTheme(ctx, req)
	})
	registerRPC[*tg.MessagesGetShareThemeBackdropsRequest](d, tlprofile.SemanticMethodMessagesGetShareThemeBackdrops, func(ctx context.Context, req *tg.MessagesGetShareThemeBackdropsRequest) (any, error) {
		return r.onMessagesGetShareThemeBackdrops(ctx, req)
	})
	registerRPC[*tg.PaymentsWithdrawStarGiftRequest](d, tlprofile.SemanticMethodPaymentsWithdrawStarGift, func(ctx context.Context, req *tg.PaymentsWithdrawStarGiftRequest) (any, error) {
		return r.onPaymentsWithdrawStarGift(ctx, req)
	})
	registerRPC[*tg.PaymentsDepositStarGiftRequest](d, tlprofile.SemanticMethodPaymentsDepositStarGift, func(ctx context.Context, req *tg.PaymentsDepositStarGiftRequest) (any, error) {
		return r.onPaymentsDepositStarGift(ctx, req)
	})
	registerRPC[*tg.PaymentsDisplayStarGiftRequest](d, tlprofile.SemanticMethodPaymentsDisplayStarGift, func(ctx context.Context, req *tg.PaymentsDisplayStarGiftRequest) (any, error) {
		return r.onPaymentsDisplayStarGift(ctx, req)
	})
	registerRPC[*tg.ContactsExportNoPaidMessagesTokenRequest](d, tlprofile.SemanticMethodContactsExportNoPaidMessagesToken, func(ctx context.Context, req *tg.ContactsExportNoPaidMessagesTokenRequest) (any, error) {
		return r.onContactsExportNoPaidMessagesToken(ctx, req)
	})
	registerRPC[*tg.ContactsImportNoPaidMessagesTokenRequest](d, tlprofile.SemanticMethodContactsImportNoPaidMessagesToken, func(ctx context.Context, req *tg.ContactsImportNoPaidMessagesTokenRequest) (any, error) {
		return r.onContactsImportNoPaidMessagesToken(ctx, req)
	})
	registerRPC[*tg.WalletSetCardStickersRequest](d, tlprofile.SemanticMethodWalletSetCardStickers, func(ctx context.Context, req *tg.WalletSetCardStickersRequest) (any, error) {
		return r.onWalletSetCardStickers(ctx, req)
	})
}
