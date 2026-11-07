package rpc

import (
	"context"
	"strings"
	"testing"

	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap/zaptest"

	appchannels "telesrv/internal/app/channels"
	appusers "telesrv/internal/app/users"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestAccountUploadWallPaperAndGet(t *testing.T) {
	ctx := WithUserID(context.Background(), 1000000001)
	files := &fakeFiles{docs: map[int64]domain.Document{}}
	router := New(Config{}, Deps{Files: files}, zaptest.NewLogger(t), clock.System)

	req := &tg.AccountUploadWallPaperRequest{
		File:     &tg.InputFile{ID: 101, Parts: 1, Name: "bg.jpg"},
		MimeType: "image/jpeg",
	}
	res, err := router.onAccountUploadWallPaper(ctx, req)
	if err != nil {
		t.Fatalf("onAccountUploadWallPaper failed: %v", err)
	}
	wp, ok := res.(*tg.WallPaper)
	if !ok {
		t.Fatalf("upload wallpaper result = %T, want *tg.WallPaper", res)
	}
	if !wp.Creator {
		t.Errorf("expected Creator=true")
	}
	if wp.Document == nil {
		t.Fatalf("expected non-nil Document")
	}

	// Lookup via account.getWallPaper
	found, err := router.onAccountGetWallPaper(router, ctx, &tg.InputWallPaper{ID: wp.ID, AccessHash: wp.AccessHash})
	if err != nil {
		t.Fatalf("getWallPaper failed: %v", err)
	}
	if found.GetID() != wp.ID {
		t.Errorf("got wallpaper id = %d, want %d", found.GetID(), wp.ID)
	}

	// Lookup via account.getMultiWallPapers
	multiRes, err := router.onAccountGetMultiWallPapers(router, ctx, []tg.InputWallPaperClass{
		&tg.InputWallPaper{ID: wp.ID, AccessHash: wp.AccessHash},
	})
	if err != nil {
		t.Fatalf("getMultiWallPapers failed: %v", err)
	}
	if len(multiRes) != 1 || multiRes[0].GetID() != wp.ID {
		t.Errorf("getMultiWallPapers = %#v, want 1 wallpaper with ID %d", multiRes, wp.ID)
	}
}

func (r *Router) onAccountGetWallPaper(router *Router, ctx context.Context, wp tg.InputWallPaperClass) (tg.WallPaperClass, error) {
	found, ok := router.lookupWallPaper(ctx, wp)
	if !ok {
		return nil, tgerr400("WALLPAPER_INVALID")
	}
	return found, nil
}

func (r *Router) onAccountGetMultiWallPapers(router *Router, ctx context.Context, wps []tg.InputWallPaperClass) ([]tg.WallPaperClass, error) {
	found := make([]tg.WallPaperClass, 0, len(wps))
	for _, input := range wps {
		wallpaper, ok := router.lookupWallPaper(ctx, input)
		if !ok {
			continue
		}
		found = append(found, wallpaper)
	}
	return found, nil
}

func TestChannelWallpaperUploadAndSet(t *testing.T) {
	ctx := context.Background()
	userStore := memory.NewUserStore()
	alice, _ := userStore.Create(ctx, domain.User{AccessHash: 1, Phone: "15550001001", FirstName: "Alice"})
	channelStore := memory.NewChannelStore()
	channels := appchannels.NewService(channelStore)
	created, err := channels.CreateChannel(ctx, alice.ID, domain.CreateChannelRequest{
		Title:     "Upload Wallpaper Channel",
		Broadcast: true,
		Date:      1700000000,
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	doc := domain.Document{
		ID:         888,
		AccessHash: 88,
		DCID:       2,
		MimeType:   "image/jpeg",
		Attributes: []domain.DocumentAttribute{
			{Kind: domain.DocAttrFilename, FileName: "wallpaper.jpg"},
			{Kind: domain.DocAttrImageSize, W: 1920, H: 1080},
		},
	}
	files := &fakeFiles{docs: map[int64]domain.Document{888: doc}}
	router := New(Config{}, Deps{
		Users:    appusers.NewService(userStore),
		Channels: channels,
		Files:    files,
	}, zaptest.NewLogger(t), clock.System)

	setReq := &tg.MessagesSetChatWallPaperRequest{
		Peer: &tg.InputPeerChannel{ChannelID: created.Channel.ID, AccessHash: created.Channel.AccessHash},
	}
	setReq.SetWallpaper(&tg.InputWallPaper{ID: 888, AccessHash: 88})
	settings := tg.WallPaperSettings{}
	settings.SetBlur(true)
	setReq.SetSettings(settings)

	got, err := router.onMessagesSetChatWallPaper(WithUserID(ctx, alice.ID), setReq)
	if err != nil {
		t.Fatalf("set chat wallpaper failed: %v", err)
	}
	updates, ok := got.(*tg.Updates)
	if !ok {
		t.Fatalf("set wallpaper got %T, want *tg.Updates", got)
	}

	var foundWP *tg.WallPaper
	for _, u := range updates.Updates {
		if pw, ok := u.(*tg.UpdatePeerWallpaper); ok {
			if w, ok := pw.GetWallpaper(); ok {
				if wp, ok := w.(*tg.WallPaper); ok {
					foundWP = wp
					break
				}
			}
		}
	}
	if foundWP == nil {
		t.Fatalf("expected *tg.WallPaper in UpdatePeerWallpaper, got none")
	}
	if foundWP.ID != 888 {
		t.Errorf("wallpaper ID = %d, want 888", foundWP.ID)
	}
	if foundWP.Document == nil {
		t.Fatalf("wallpaper Document is nil")
	}

	view, err := channels.GetChannel(ctx, alice.ID, created.Channel.ID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	if view.Channel.Wallpaper == nil || view.Channel.Wallpaper.Document == nil {
		t.Fatalf("stored channel wallpaper document is missing")
	}
	if view.Channel.Wallpaper.Document.ID != 888 {
		t.Errorf("stored document id = %d, want 888", view.Channel.Wallpaper.Document.ID)
	}

	full := tgChannelFull(view)
	fullWP, ok := full.GetWallpaper()
	if !ok {
		t.Fatalf("channelFull missing wallpaper")
	}
	fullWallPaper, ok := fullWP.(*tg.WallPaper)
	if !ok || fullWallPaper.ID != 888 {
		t.Fatalf("channelFull wallpaper = %#v, want WallPaper with ID 888", fullWP)
	}
}

func TestChannelUniqueGiftThemeAndEmojiStatus(t *testing.T) {
	ctx := context.Background()
	userStore := memory.NewUserStore()
	alice, _ := userStore.Create(ctx, domain.User{AccessHash: 1, Phone: "15550001001", FirstName: "Alice"})
	bob, _ := userStore.Create(ctx, domain.User{AccessHash: 2, Phone: "15550001002", FirstName: "Bob"})
	channelStore := memory.NewChannelStore()
	channels := appchannels.NewService(channelStore)
	created, err := channels.CreateChannel(ctx, alice.ID, domain.CreateChannelRequest{
		Title:     "Gift Wearing Channel",
		Broadcast: true,
		Date:      1700000000,
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	gift := collectibleEmojiTestGift(alice.ID)
	gifts := &collectibleEmojiGiftService{
		gifts: map[int64]domain.UniqueStarGift{
			gift.ID: gift,
		},
	}
	router := New(Config{}, Deps{
		Users:    appusers.NewService(userStore),
		Channels: channels,
		Gifts:    gifts,
	}, zaptest.NewLogger(t), clock.System)

	// 1. Wear unique gift as channel chat theme (messages.setChatTheme)
	themeReq := &tg.MessagesSetChatThemeRequest{
		Peer:  &tg.InputPeerChannel{ChannelID: created.Channel.ID, AccessHash: created.Channel.AccessHash},
		Theme: &tg.InputChatThemeUniqueGift{Slug: gift.Slug},
	}
	themeGot, err := router.onMessagesSetChatTheme(WithUserID(ctx, alice.ID), themeReq)
	if err != nil {
		t.Fatalf("set chat theme with unique gift failed: %v", err)
	}
	themeUpdates, ok := themeGot.(*tg.Updates)
	if !ok {
		t.Fatalf("themeUpdates = %T, want *tg.Updates", themeGot)
	}
	var themeWP *tg.WallPaper
	for _, u := range themeUpdates.Updates {
		if pw, ok := u.(*tg.UpdatePeerWallpaper); ok {
			if w, ok := pw.GetWallpaper(); ok {
				if wp, ok := w.(*tg.WallPaper); ok {
					themeWP = wp
					break
				}
			}
		}
	}
	if themeWP == nil {
		t.Fatalf("expected *tg.WallPaper in theme updates, got none")
	}
	if themeWP.Slug != gift.Slug {
		t.Errorf("wallpaper slug = %q, want %q", themeWP.Slug, gift.Slug)
	}
	if !themeWP.Pattern {
		t.Errorf("expected Pattern=true")
	}

	// 2. Wear unique gift as channel emoji status (channels.updateEmojiStatus)
	statusReq := &tg.ChannelsUpdateEmojiStatusRequest{
		Channel:     &tg.InputChannel{ChannelID: created.Channel.ID, AccessHash: created.Channel.AccessHash},
		EmojiStatus: &tg.InputEmojiStatusCollectible{CollectibleID: gift.ID},
	}
	statusGot, err := router.onChannelsUpdateEmojiStatus(WithUserID(ctx, alice.ID), statusReq)
	if err != nil {
		t.Fatalf("update channel emoji status failed: %v", err)
	}
	if statusGot == nil {
		t.Fatalf("expected non-nil status updates")
	}

	view, err := channels.GetChannel(ctx, alice.ID, created.Channel.ID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	if view.Channel.EmojiStatus.Collectible.CollectibleID != gift.ID {
		t.Errorf("channel collectible id = %d, want %d", view.Channel.EmojiStatus.Collectible.CollectibleID, gift.ID)
	}
	tgStatus := tgChannelEmojiStatus(view.Channel.EmojiStatus)
	collectibleStatus, ok := tgStatus.(*tg.EmojiStatusCollectible)
	if !ok {
		t.Fatalf("tgChannelEmojiStatus = %T, want *tg.EmojiStatusCollectible", tgStatus)
	}
	if collectibleStatus.CollectibleID != gift.ID || collectibleStatus.Title != gift.Title {
		t.Errorf("collectibleStatus = %#v, want ID=%d Title=%s", collectibleStatus, gift.ID, gift.Title)
	}

	// Non-owner bob tries to set alice's gift as emoji status on his channel
	bobChannel, err := channels.CreateChannel(ctx, bob.ID, domain.CreateChannelRequest{
		Title:     "Bob Channel",
		Broadcast: true,
		Date:      1700000000,
	})
	if err != nil {
		t.Fatalf("create bob channel: %v", err)
	}
	bobReq := &tg.ChannelsUpdateEmojiStatusRequest{
		Channel:     &tg.InputChannel{ChannelID: bobChannel.Channel.ID, AccessHash: bobChannel.Channel.AccessHash},
		EmojiStatus: &tg.InputEmojiStatusCollectible{CollectibleID: gift.ID},
	}
	_, err = router.onChannelsUpdateEmojiStatus(WithUserID(ctx, bob.ID), bobReq)
	if err == nil || !strings.Contains(err.Error(), "COLLECTIBLE_INVALID") {
		t.Fatalf("expected COLLECTIBLE_INVALID for non-owner, got %v", err)
	}

	// 3. Get unique gift chat themes for alice
	themesRes, err := router.onAccountGetUniqueGiftChatThemes(WithUserID(ctx, alice.ID), &tg.AccountGetUniqueGiftChatThemesRequest{})
	if err != nil {
		t.Fatalf("onAccountGetUniqueGiftChatThemes failed: %v", err)
	}
	themesList, ok := themesRes.(*tg.AccountChatThemes)
	if !ok || len(themesList.Themes) != 1 {
		t.Fatalf("themes list = %#v, want 1 theme", themesRes)
	}
	uniqueTheme, ok := themesList.Themes[0].(*tg.ChatThemeUniqueGift)
	if !ok {
		t.Fatalf("theme = %T, want *tg.ChatThemeUniqueGift", themesList.Themes[0])
	}
	if len(uniqueTheme.ThemeSettings) != 5 {
		t.Errorf("got %d theme settings, want 5", len(uniqueTheme.ThemeSettings))
	}

	// 4. Update channel color with background emoji symbol
	colorReq := &tg.ChannelsUpdateColorRequest{
		Channel:    &tg.InputChannel{ChannelID: created.Channel.ID, AccessHash: created.Channel.AccessHash},
		ForProfile: true,
	}
	colorReq.SetBackgroundEmojiID(gift.Pattern.Document.ID)
	colorReq.SetColor(5)
	colorGot, err := router.onChannelsUpdateColor(WithUserID(ctx, alice.ID), colorReq)
	if err != nil {
		t.Fatalf("update channel color failed: %v", err)
	}
	if colorGot == nil {
		t.Fatalf("expected non-nil color updates")
	}

	view, err = channels.GetChannel(ctx, alice.ID, created.Channel.ID)
	if err != nil {
		t.Fatalf("get channel after color: %v", err)
	}
	if view.Channel.ProfileColor.BackgroundEmojiID != gift.Pattern.Document.ID {
		t.Errorf("background emoji id = %d, want %d", view.Channel.ProfileColor.BackgroundEmojiID, gift.Pattern.Document.ID)
	}
	peerColor := tgPeerColor(view.Channel.ProfileColor)
	if pc, ok := peerColor.(*tg.PeerColor); !ok || pc.BackgroundEmojiID != gift.Pattern.Document.ID {
		t.Errorf("tgPeerColor = %#v, want BackgroundEmojiID %d", peerColor, gift.Pattern.Document.ID)
	}
}
