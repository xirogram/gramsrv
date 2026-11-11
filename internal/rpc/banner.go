package rpc

import (
	"context"

	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tlprofile"

	"telesrv/internal/domain"
)

// registerBanner wires the banner.* fork extension RPCs. These are custom XiroGram
// methods (not part of the canonical Telegram schema): stock clients never send
// them, other forks ignore them, and the XiroGram client fork uses them to manage
// the TikTok-style profile banner (custom > gift priority is decided client-side).
func (r *Router) registerBanner(d *tlprofile.Dispatcher) {
	registerRPC[*tg.PhotosSaveProfileBannerRequest](d, tlprofile.SemanticMethodPhotosSaveProfileBanner, func(ctx context.Context, layerRequest *tg.PhotosSaveProfileBannerRequest) (any, error) {
		return r.onPhotosSaveProfileBanner(ctx, layerRequest)
	})
	registerRPC[*tg.PhotosGetProfileBannerRequest](d, tlprofile.SemanticMethodPhotosGetProfileBanner, func(ctx context.Context, layerRequest *tg.PhotosGetProfileBannerRequest) (any, error) {
		return r.onPhotosGetProfileBanner(ctx, layerRequest)
	})
	registerRPC[*tg.PhotosRemoveProfileBannerRequest](d, tlprofile.SemanticMethodPhotosRemoveProfileBanner, func(ctx context.Context, layerRequest *tg.PhotosRemoveProfileBannerRequest) (any, error) {
		return r.onPhotosRemoveProfileBanner(ctx, layerRequest)
	})
}

func (r *Router) onPhotosSaveProfileBanner(ctx context.Context, req *tg.PhotosSaveProfileBannerRequest) (bool, error) {
	if r.deps.Files == nil {
		return false, notImplementedErr()
	}
	userID, ok, err := r.currentUserID(ctx)
	if err != nil {
		return false, internalErr()
	}
	if !ok || userID == 0 {
		return false, photoInvalidErr()
	}
	upload, mediaFlags, err := parseProfilePhotoUpload(req)
	if err != nil {
		return false, err
	}
	if mediaFlags == 0 {
		return false, photoInvalidErr()
	}
	photo, err := r.createProfilePhoto(ctx, userID, upload)
	if err != nil {
		return false, err
	}
	if _, found, err := r.deps.Files.SetCurrentProfilePhotoKind(ctx, domain.PeerTypeUser, userID, domain.ProfilePhotoKindBanner, photo.ID, int(r.clock.Now().Unix())); err != nil {
		return false, internalErr()
	} else if !found {
		return false, photoInvalidErr()
	}
	r.invalidateRPCProjectionForPeer(userID, domain.Peer{Type: domain.PeerTypeUser, ID: userID})
	return true, nil
}

func (r *Router) onPhotosGetProfileBanner(ctx context.Context, req *tg.PhotosGetProfileBannerRequest) (*tg.PhotosPhoto, error) {
	if r.deps.Files == nil || r.deps.Users == nil {
		return nil, notImplementedErr()
	}
	viewerID, ok, err := r.currentUserID(ctx)
	if err != nil {
		return nil, internalErr()
	}
	if !ok || viewerID == 0 {
		return nil, photoInvalidErr()
	}
	target, found, err := r.userFromInput(ctx, viewerID, req.UserID)
	if err != nil {
		return nil, internalErr()
	}
	if !found || target.ID == 0 {
		return nil, userIDInvalidErr()
	}
	photo, found, err := r.deps.Files.CurrentProfilePhotoKind(ctx, domain.PeerTypeUser, target.ID, domain.ProfilePhotoKindBanner)
	if err != nil {
		return nil, internalErr()
	}
	if !found || photo.ID == 0 {
		return r.photosPhotoForUser(ctx, viewerID, target.ID, domain.Photo{}), nil
	}
	return r.photosPhotoForUser(ctx, viewerID, target.ID, photo), nil
}

func (r *Router) onPhotosRemoveProfileBanner(ctx context.Context, req *tg.PhotosRemoveProfileBannerRequest) (bool, error) {
	if r.deps.Files == nil {
		return false, notImplementedErr()
	}
	userID, ok, err := r.currentUserID(ctx)
	if err != nil {
		return false, internalErr()
	}
	if !ok || userID == 0 {
		return false, photoInvalidErr()
	}
	if cur, found, err := r.deps.Files.CurrentProfilePhotoKind(ctx, domain.PeerTypeUser, userID, domain.ProfilePhotoKindBanner); err == nil && found && cur.ID != 0 {
		_, _ = r.deps.Files.DeleteProfilePhotosKind(ctx, domain.PeerTypeUser, userID, domain.ProfilePhotoKindBanner, []int64{cur.ID})
	}
	r.invalidateRPCProjectionForPeer(userID, domain.Peer{Type: domain.PeerTypeUser, ID: userID})
	return true, nil
}
