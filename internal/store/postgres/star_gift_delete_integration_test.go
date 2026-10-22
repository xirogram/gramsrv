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
