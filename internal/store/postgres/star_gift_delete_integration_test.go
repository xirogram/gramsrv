package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"telesrv/internal/domain"
)

// TestStarGiftDeleteRemovesGiftFromProfilePostgres pins payments.deleteStarGift:
// the gift leaves the profile through the deleted lifecycle state, the row
// survives for the audit trail and a second delete is refused.
func TestStarGiftDeleteRemovesGiftFromProfilePostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	suffix := randomSuffix(t)
	now := int(time.Now().Unix())
	users := NewUserStore(pool)
	owner := createTestUser(t, ctx, users, "+1882"+suffix+"01", "DeleteOwner", "")
	stranger := createTestUser(t, ctx, users, "+1882"+suffix+"02", "DeleteStranger", "")
	revisionID, giftID := ratingTestCatalogRevision(t, pool)

	var savedID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO peer_star_gifts (owner_peer_type, owner_peer_id, msg_id, gift_id, gift_date, catalog_revision_id, lifecycle_status, converted)
VALUES ('user', $1, 1, $2, 0, $3, 'active', false) RETURNING id`,
		owner.ID, giftID, revisionID).Scan(&savedID); err != nil {
		t.Fatalf("seed saved gift: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM star_gift_collection_items WHERE saved_gift_id = $1`, savedID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM peer_star_gifts WHERE id = $1`, savedID)
	})

	lifecycle := NewStarGiftLifecycleStore(pool, NewMessageStore(pool), 1_000_000)
	ref := domain.SavedStarGiftRef{Owner: domain.Peer{Type: domain.PeerTypeUser, ID: owner.ID}, MsgID: 1}

	// A stranger may not delete somebody else's gift.
	if _, err := lifecycle.DeleteStarGift(ctx, domain.StarGiftDeleteRequest{
		UserID: stranger.ID, Ref: ref, Date: now,
	}); !errors.Is(err, domain.ErrStarGiftOwnerInvalid) {
		t.Fatalf("stranger delete err = %v, want ErrStarGiftOwnerInvalid", err)
	}

	result, err := lifecycle.DeleteStarGift(ctx, domain.StarGiftDeleteRequest{UserID: owner.ID, Ref: ref, Date: now})
	if err != nil {
		t.Fatalf("delete star gift: %v", err)
	}
	if !result.Deleted || result.SavedGiftID != savedID || result.UniqueGiftID != 0 {
		t.Fatalf("delete result = %+v", result)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT lifecycle_status FROM peer_star_gifts WHERE id = $1`, savedID).Scan(&status); err != nil {
		t.Fatalf("read lifecycle status: %v", err)
	}
	if status != string(domain.StarGiftLifecycleDeleted) {
		t.Fatalf("lifecycle status = %q, want %q", status, domain.StarGiftLifecycleDeleted)
	}
	// The deleted gift must not be projected as a saved gift any more.
	saved, found, err := NewStarGiftStore(pool).GetByRef(ctx, ref)
	if err != nil {
		t.Fatalf("read saved gift: %v", err)
	}
	if found && saved.LifecycleStatus.Live() {
		t.Fatalf("deleted gift is still live: %+v", saved)
	}
	// A second delete is refused instead of silently succeeding.
	if _, err := lifecycle.DeleteStarGift(ctx, domain.StarGiftDeleteRequest{UserID: owner.ID, Ref: ref, Date: now}); !errors.Is(err, domain.ErrStarGiftDeleteUnavailable) {
		t.Fatalf("second delete err = %v, want ErrStarGiftDeleteUnavailable", err)
	}
}

// TestStarGiftDeleteRefusesUpgradedGiftPostgres pins that a gift which was
// upgraded into a collectible (NFT) is not deletable: destroying a collectible
// is a different operation with its own financial rules.
func TestStarGiftDeleteRefusesUpgradedGiftPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	suffix := randomSuffix(t)
	now := int(time.Now().Unix())
	users := NewUserStore(pool)
	owner := createTestUser(t, ctx, users, "+1883"+suffix+"01", "UpgradedOwner", "")
	sender := createTestUser(t, ctx, users, "+1883"+suffix+"02", "UpgradedSender", "")
	ownerPeer := domain.Peer{Type: domain.PeerTypeUser, ID: owner.ID}

	gifts := NewStarGiftStore(pool)
	baseDocumentID := time.Now().UnixNano() & 0x7ffffffffffff000
	entry, err := gifts.CreateCatalogRevision(ctx, domain.StarGiftCatalogWrite{
		Title: "Delete " + suffix, Stars: 50, ConvertStars: 20, Enabled: true,
		Document: collectibleTestDocument(baseDocumentID, "delete.tgs"),
		Blob:     collectibleTestBlob(baseDocumentID, "delete"), Animation: collectibleTestAnimation("delete.tgs"),
		Actor: "integration", CommandID: "delete-catalog-" + suffix,
	})
	if err != nil {
		t.Fatalf("create delete catalog: %v", err)
	}
	if _, err := gifts.PublishCollectibleRevision(ctx, domain.StarGiftCollectibleWrite{
		GiftID: entry.Gift.ID, UpgradeStars: 100, SupplyTotal: 5, SlugPrefix: "del-" + suffix,
		Models: []domain.StarGiftCollectibleAttribute{{
			Kind: domain.StarGiftCollectibleModel, Name: "Base", RarityKind: domain.StarGiftRarityPermille, RarityPermille: 1000,
			Document: collectibleTestDocumentPtr(baseDocumentID+1, "model.tgs"), Blob: collectibleTestBlobPtr(baseDocumentID+1, "model"), Animation: collectibleTestAnimationPtr("model.tgs"),
		}},
		Patterns: []domain.StarGiftCollectibleAttribute{{
			Kind: domain.StarGiftCollectiblePattern, Name: "Orbit", RarityKind: domain.StarGiftRarityPermille, RarityPermille: 1000,
			Document: collectibleTestPatternDocumentPtr(baseDocumentID+2, "pattern.tgs"), Blob: collectibleTestBlobPtr(baseDocumentID+2, "pattern"), Animation: collectibleTestAnimationPtr("pattern.tgs"),
		}},
		Backdrops: []domain.StarGiftCollectibleAttribute{{
			Kind: domain.StarGiftCollectibleBackdrop, Name: "Night", BackdropID: 91, RarityKind: domain.StarGiftRarityPermille, RarityPermille: 1000,
			CenterColor: 0x112233, EdgeColor: 0x223344, PatternColor: 0x334455, TextColor: 0xffffff,
		}, {
			Kind: domain.StarGiftCollectibleBackdrop, Name: "Day", BackdropID: 92, RarityKind: domain.StarGiftRarityPermille, RarityPermille: 500,
			CenterColor: 0xaabbcc, EdgeColor: 0x778899, PatternColor: 0xddeeff, TextColor: 0x111111,
		}},
		Actor: "integration", CommandID: "delete-pool-" + suffix,
	}); err != nil {
		t.Fatalf("publish delete pool: %v", err)
	}

	messages := NewMessageStore(pool)
	saved := createCollectibleSavedGift(t, ctx, messages, gifts, entry.Gift, domain.SavedStarGift{
		Owner: ownerPeer, FromUserID: sender.ID, GiftID: entry.Gift.ID, RevisionID: entry.Gift.RevisionID,
		Date: now, ConvertStars: 20, Message: "upgraded gift",
	})
	stars := NewStarsStore(pool)
	if _, _, err := stars.EnsureGrant(ctx, owner.ID, 1000, now); err != nil {
		t.Fatalf("grant upgrade stars: %v", err)
	}
	upgrades := NewStarGiftUpgradeStore(pool, messages)
	upgraded, err := upgrades.UpgradeStarGift(ctx, domain.StarGiftUpgradeRequest{
		UserID: owner.ID, Ref: domain.SavedStarGiftRef{Owner: ownerPeer, MsgID: saved.MsgID},
		ChargeStars: 100, FormID: 8801, CommandKey: "delete-upgrade-" + suffix, Date: now + 1,
	})
	if err != nil {
		t.Fatalf("upgrade gift: %v", err)
	}
	if upgraded.Unique.ID <= 0 {
		t.Fatalf("upgrade result = %+v", upgraded)
	}

	lifecycle := NewStarGiftLifecycleStore(pool, messages, 1_000_000)
	if _, err := lifecycle.DeleteStarGift(ctx, domain.StarGiftDeleteRequest{
		UserID: owner.ID, Ref: domain.SavedStarGiftRef{Owner: ownerPeer, MsgID: saved.MsgID}, Date: now + 2,
	}); !errors.Is(err, domain.ErrStarGiftDeleteUnavailable) {
		t.Fatalf("delete upgraded gift err = %v, want ErrStarGiftDeleteUnavailable", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT lifecycle_status FROM peer_star_gifts WHERE id = $1`, saved.ID).Scan(&status); err != nil {
		t.Fatalf("read lifecycle status: %v", err)
	}
	if status != string(domain.StarGiftLifecycleActive) {
		t.Fatalf("upgraded gift lifecycle = %q, want %q", status, domain.StarGiftLifecycleActive)
	}
}
