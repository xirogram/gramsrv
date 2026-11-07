package rpc

import (
	"context"

	"github.com/iamxvbaba/td/tg"

	"telesrv/internal/compat/tdesktop"
	"telesrv/internal/domain"
)

func (r *Router) domainWallpaperFromSetChatWallPaper(ctx context.Context, req *tg.MessagesSetChatWallPaperRequest) (*domain.Wallpaper, error) {
	if req == nil || req.Revert {
		return nil, nil
	}
	input, ok := req.GetWallpaper()
	if !ok {
		return nil, nil
	}
	settings, hasSettings := req.GetSettings()
	return r.domainWallpaperFromInput(ctx, input, settings, hasSettings)
}

func (r *Router) domainWallpaperFromInput(ctx context.Context, input tg.InputWallPaperClass, settings tg.WallPaperSettings, hasSettings bool) (*domain.Wallpaper, error) {
	switch in := input.(type) {
	case *tg.InputWallPaperNoFile:
		wp := &domain.Wallpaper{ID: in.ID, NoFile: true}
		if hasSettings {
			wp.Settings = domainWallpaperSettings(settings)
		}
		return wp, nil
	case *tg.InputWallPaper:
		if found, ok := tdesktop.LookupWallPaper(input); ok {
			wp := domainWallpaperFromTG(found)
			if wp == nil {
				return nil, tgerr400("WALLPAPER_INVALID")
			}
			if hasSettings {
				wp.Settings = domainWallpaperSettings(settings)
			}
			return wp, nil
		}
		if r != nil && r.deps.Files != nil && in.ID != 0 {
			doc, foundDoc, err := r.deps.Files.GetDocument(ctx, in.ID)
			if err == nil && foundDoc && (in.AccessHash == 0 || doc.AccessHash == in.AccessHash) {
				wp := &domain.Wallpaper{
					ID:         in.ID,
					AccessHash: doc.AccessHash,
					Document:   &doc,
				}
				if hasSettings {
					wp.Settings = domainWallpaperSettings(settings)
				}
				return wp, nil
			}
		}
		if r != nil && r.deps.Gifts != nil && in.ID != 0 {
			gift, foundGift, err := r.deps.Gifts.UniqueByID(ctx, in.ID)
			if err == nil && foundGift && gift.Pattern.Document != nil {
				wp := &domain.Wallpaper{
					ID:         gift.Pattern.Document.ID,
					AccessHash: gift.Pattern.Document.AccessHash,
					Slug:       gift.Slug,
					Pattern:    true,
					Document:   gift.Pattern.Document,
					Settings: domain.WallpaperSettings{
						HasBackgroundColor:       true,
						BackgroundColor:          gift.Backdrop.CenterColor,
						HasSecondBackgroundColor: true,
						SecondBackgroundColor:    gift.Backdrop.EdgeColor,
						HasThirdBackgroundColor:  true,
						ThirdBackgroundColor:     gift.Backdrop.PatternColor,
						HasFourthBackgroundColor: true,
						FourthBackgroundColor:    gift.Backdrop.TextColor,
					},
				}
				if hasSettings {
					wp.Settings = domainWallpaperSettings(settings)
				}
				return wp, nil
			}
		}
		return nil, tgerr400("WALLPAPER_INVALID")
	case *tg.InputWallPaperSlug:
		if found, ok := tdesktop.LookupWallPaper(input); ok {
			wp := domainWallpaperFromTG(found)
			if wp == nil {
				return nil, tgerr400("WALLPAPER_INVALID")
			}
			if hasSettings {
				wp.Settings = domainWallpaperSettings(settings)
			}
			return wp, nil
		}
		if r != nil && r.deps.Gifts != nil && in.Slug != "" {
			gift, foundGift, err := r.deps.Gifts.UniqueBySlug(ctx, in.Slug)
			if err == nil && foundGift && gift.Pattern.Document != nil {
				wp := &domain.Wallpaper{
					ID:         gift.Pattern.Document.ID,
					AccessHash: gift.Pattern.Document.AccessHash,
					Slug:       gift.Slug,
					Pattern:    true,
					Document:   gift.Pattern.Document,
					Settings: domain.WallpaperSettings{
						HasBackgroundColor:       true,
						BackgroundColor:          gift.Backdrop.CenterColor,
						HasSecondBackgroundColor: true,
						SecondBackgroundColor:    gift.Backdrop.EdgeColor,
						HasThirdBackgroundColor:  true,
						ThirdBackgroundColor:     gift.Backdrop.PatternColor,
						HasFourthBackgroundColor: true,
						FourthBackgroundColor:    gift.Backdrop.TextColor,
					},
				}
				if hasSettings {
					wp.Settings = domainWallpaperSettings(settings)
				}
				return wp, nil
			}
		}
		return nil, tgerr400("WALLPAPER_INVALID")
	default:
		return nil, inputConstructorInvalidErr()
	}
}

func domainWallpaperFromTG(input tg.WallPaperClass) *domain.Wallpaper {
	switch wp := input.(type) {
	case *tg.WallPaperNoFile:
		out := &domain.Wallpaper{
			ID:       wp.ID,
			NoFile:   true,
			Default:  wp.Default,
			Dark:     wp.Dark,
			Settings: domainWallpaperSettingsFromOptional(wp.GetSettings()),
		}
		return out
	case *tg.WallPaper:
		out := &domain.Wallpaper{
			ID:         wp.ID,
			AccessHash: wp.AccessHash,
			Slug:       wp.Slug,
			Default:    wp.Default,
			Pattern:    wp.Pattern,
			Dark:       wp.Dark,
			Settings:   domainWallpaperSettingsFromOptional(wp.GetSettings()),
		}
		return out
	default:
		return nil
	}
}

func tgWallpaper(wp *domain.Wallpaper) tg.WallPaperClass {
	if wp == nil {
		return nil
	}
	if wp.NoFile {
		out := &tg.WallPaperNoFile{ID: wp.ID}
		out.SetDefault(wp.Default)
		out.SetDark(wp.Dark)
		if !wp.Settings.Empty() {
			out.SetSettings(tgWallpaperSettings(wp.Settings))
		}
		return out
	}
	if wp.Document != nil {
		out := &tg.WallPaper{
			ID:         wp.ID,
			AccessHash: wp.AccessHash,
			Slug:       wp.Slug,
			Default:    wp.Default,
			Pattern:    wp.Pattern,
			Dark:       wp.Dark,
			Document:   tgDocument(*wp.Document),
		}
		if !wp.Settings.Empty() {
			out.SetSettings(tgWallpaperSettings(wp.Settings))
		}
		return out
	}
	if wp.ID != 0 && wp.AccessHash != 0 {
		if found, ok := tdesktop.LookupWallPaper(&tg.InputWallPaper{ID: wp.ID, AccessHash: wp.AccessHash}); ok {
			return tgWallpaperWithSettings(found, wp.Settings)
		}
	}
	if wp.Slug != "" {
		if found, ok := tdesktop.LookupWallPaper(&tg.InputWallPaperSlug{Slug: wp.Slug}); ok {
			return tgWallpaperWithSettings(found, wp.Settings)
		}
	}
	out := &tg.WallPaperNoFile{ID: wp.ID}
	out.SetDefault(wp.Default)
	out.SetDark(wp.Dark)
	if !wp.Settings.Empty() {
		out.SetSettings(tgWallpaperSettings(wp.Settings))
	}
	return out
}

func tgWallpaperWithSettings(input tg.WallPaperClass, settings domain.WallpaperSettings) tg.WallPaperClass {
	switch wp := input.(type) {
	case *tg.WallPaperNoFile:
		out := *wp
		if !settings.Empty() {
			out.SetSettings(tgWallpaperSettings(settings))
		}
		return &out
	case *tg.WallPaper:
		out := *wp
		if !settings.Empty() {
			out.SetSettings(tgWallpaperSettings(settings))
		}
		return &out
	default:
		return input
	}
}

func domainWallpaperSettingsFromOptional(settings tg.WallPaperSettings, ok bool) domain.WallpaperSettings {
	if !ok {
		return domain.WallpaperSettings{}
	}
	return domainWallpaperSettings(settings)
}

func domainWallpaperSettings(in tg.WallPaperSettings) domain.WallpaperSettings {
	out := domain.WallpaperSettings{
		Blur:   in.Blur || in.GetBlur(),
		Motion: in.Motion || in.GetMotion(),
	}
	if v, ok := in.GetBackgroundColor(); ok {
		out.HasBackgroundColor = true
		out.BackgroundColor = v
	} else if in.BackgroundColor != 0 {
		out.HasBackgroundColor = true
		out.BackgroundColor = in.BackgroundColor
	}
	if v, ok := in.GetSecondBackgroundColor(); ok {
		out.HasSecondBackgroundColor = true
		out.SecondBackgroundColor = v
	} else if in.SecondBackgroundColor != 0 {
		out.HasSecondBackgroundColor = true
		out.SecondBackgroundColor = in.SecondBackgroundColor
	}
	if v, ok := in.GetThirdBackgroundColor(); ok {
		out.HasThirdBackgroundColor = true
		out.ThirdBackgroundColor = v
	} else if in.ThirdBackgroundColor != 0 {
		out.HasThirdBackgroundColor = true
		out.ThirdBackgroundColor = in.ThirdBackgroundColor
	}
	if v, ok := in.GetFourthBackgroundColor(); ok {
		out.HasFourthBackgroundColor = true
		out.FourthBackgroundColor = v
	} else if in.FourthBackgroundColor != 0 {
		out.HasFourthBackgroundColor = true
		out.FourthBackgroundColor = in.FourthBackgroundColor
	}
	if v, ok := in.GetIntensity(); ok {
		out.HasIntensity = true
		out.Intensity = v
	} else if in.Intensity != 0 {
		out.HasIntensity = true
		out.Intensity = in.Intensity
	}
	if v, ok := in.GetRotation(); ok {
		out.HasRotation = true
		out.Rotation = v
	} else if in.Rotation != 0 {
		out.HasRotation = true
		out.Rotation = in.Rotation
	}
	if v, ok := in.GetEmoticon(); ok {
		out.HasEmoticon = true
		out.Emoticon = v
	} else if in.Emoticon != "" {
		out.HasEmoticon = true
		out.Emoticon = in.Emoticon
	}
	return out
}

func tgWallpaperSettings(in domain.WallpaperSettings) tg.WallPaperSettings {
	var out tg.WallPaperSettings
	out.SetBlur(in.Blur)
	out.SetMotion(in.Motion)
	if in.HasBackgroundColor {
		out.SetBackgroundColor(in.BackgroundColor)
	}
	if in.HasSecondBackgroundColor {
		out.SetSecondBackgroundColor(in.SecondBackgroundColor)
	}
	if in.HasThirdBackgroundColor {
		out.SetThirdBackgroundColor(in.ThirdBackgroundColor)
	}
	if in.HasFourthBackgroundColor {
		out.SetFourthBackgroundColor(in.FourthBackgroundColor)
	}
	if in.HasIntensity {
		out.SetIntensity(in.Intensity)
	}
	if in.HasRotation {
		out.SetRotation(in.Rotation)
	}
	if in.HasEmoticon {
		out.SetEmoticon(in.Emoticon)
	}
	return out
}

func tgUpdatePeerWallpaper(peer domain.Peer, wallpaper *domain.Wallpaper) *tg.UpdatePeerWallpaper {
	update := &tg.UpdatePeerWallpaper{Peer: tgPeer(peer)}
	if wp := tgWallpaper(wallpaper); wp != nil {
		update.SetWallpaper(wp)
	}
	return update
}

func (r *Router) lookupWallPaper(ctx context.Context, input tg.InputWallPaperClass) (tg.WallPaperClass, bool) {
	if found, ok := tdesktop.LookupWallPaper(input); ok {
		return found, true
	}
	switch in := input.(type) {
	case *tg.InputWallPaper:
		if r != nil && r.deps.Files != nil && in.ID != 0 {
			doc, found, err := r.deps.Files.GetDocument(ctx, in.ID)
			if err == nil && found && (in.AccessHash == 0 || doc.AccessHash == in.AccessHash) {
				wp := &tg.WallPaper{
					ID:         doc.ID,
					AccessHash: doc.AccessHash,
					Creator:    true,
					Document:   tgDocument(doc),
				}
				wp.SetFlags()
				return wp, true
			}
		}
		if r != nil && r.deps.Gifts != nil && in.ID != 0 {
			gift, found, err := r.deps.Gifts.UniqueByID(ctx, in.ID)
			if err == nil && found && gift.Pattern.Document != nil {
				wp := &tg.WallPaper{
					ID:         gift.Pattern.Document.ID,
					AccessHash: gift.Pattern.Document.AccessHash,
					Slug:       gift.Slug,
					Pattern:    true,
					Document:   tgDocument(*gift.Pattern.Document),
					Settings: tgWallpaperSettings(domain.WallpaperSettings{
						HasBackgroundColor:       true,
						BackgroundColor:          gift.Backdrop.CenterColor,
						HasSecondBackgroundColor: true,
						SecondBackgroundColor:    gift.Backdrop.EdgeColor,
						HasThirdBackgroundColor:  true,
						ThirdBackgroundColor:     gift.Backdrop.PatternColor,
						HasFourthBackgroundColor: true,
						FourthBackgroundColor:    gift.Backdrop.TextColor,
					}),
				}
				wp.SetFlags()
				return wp, true
			}
		}
	case *tg.InputWallPaperSlug:
		if r != nil && r.deps.Gifts != nil && in.Slug != "" {
			gift, found, err := r.deps.Gifts.UniqueBySlug(ctx, in.Slug)
			if err == nil && found && gift.Pattern.Document != nil {
				wp := &tg.WallPaper{
					ID:         gift.Pattern.Document.ID,
					AccessHash: gift.Pattern.Document.AccessHash,
					Slug:       gift.Slug,
					Pattern:    true,
					Document:   tgDocument(*gift.Pattern.Document),
					Settings: tgWallpaperSettings(domain.WallpaperSettings{
						HasBackgroundColor:       true,
						BackgroundColor:          gift.Backdrop.CenterColor,
						HasSecondBackgroundColor: true,
						SecondBackgroundColor:    gift.Backdrop.EdgeColor,
						HasThirdBackgroundColor:  true,
						ThirdBackgroundColor:     gift.Backdrop.PatternColor,
						HasFourthBackgroundColor: true,
						FourthBackgroundColor:    gift.Backdrop.TextColor,
					}),
				}
				wp.SetFlags()
				return wp, true
			}
		}
	}
	return nil, false
}

func tgChatThemeUniqueGift(g domain.UniqueStarGift) *tg.ChatThemeUniqueGift {
	var wp tg.WallPaperClass
	if g.Pattern.Document != nil {
		w := &tg.WallPaper{
			ID:         g.Pattern.Document.ID,
			AccessHash: g.Pattern.Document.AccessHash,
			Slug:       g.Slug,
			Pattern:    true,
			Document:   tgDocument(*g.Pattern.Document),
			Settings: tgWallpaperSettings(domain.WallpaperSettings{
				HasBackgroundColor:       true,
				BackgroundColor:          g.Backdrop.CenterColor,
				HasSecondBackgroundColor: true,
				SecondBackgroundColor:    g.Backdrop.EdgeColor,
				HasThirdBackgroundColor:  true,
				ThirdBackgroundColor:     g.Backdrop.PatternColor,
				HasFourthBackgroundColor: true,
				FourthBackgroundColor:    g.Backdrop.TextColor,
			}),
		}
		w.SetFlags()
		wp = w
	}

	baseThemes := []struct {
		make func() tg.BaseThemeClass
		dark bool
	}{
		{func() tg.BaseThemeClass { return &tg.BaseThemeClassic{} }, false},
		{func() tg.BaseThemeClass { return &tg.BaseThemeDay{} }, false},
		{func() tg.BaseThemeClass { return &tg.BaseThemeNight{} }, true},
		{func() tg.BaseThemeClass { return &tg.BaseThemeTinted{} }, true},
		{func() tg.BaseThemeClass { return &tg.BaseThemeArctic{} }, false},
	}

	themeSettings := make([]tg.ThemeSettings, 0, len(baseThemes))
	for _, b := range baseThemes {
		ts := tg.ThemeSettings{
			BaseTheme:   b.make(),
			AccentColor: g.Backdrop.CenterColor,
		}
		ts.SetOutboxAccentColor(g.Backdrop.EdgeColor)
		ts.SetMessageColors([]int{g.Backdrop.CenterColor, g.Backdrop.EdgeColor})
		if wp != nil {
			ts.SetWallpaper(wp)
		}
		ts.SetFlags()
		themeSettings = append(themeSettings, ts)
	}

	return &tg.ChatThemeUniqueGift{
		Gift:          tgUniqueStarGift(g),
		ThemeSettings: themeSettings,
	}
}

