package wallet

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"telesrv/internal/domain"
	"testing"
	"time"
)

func TestTransactionsRequiresHistorySource(t *testing.T) {
	s := &Service{}
	_, err := s.Transactions(context.Background(), 42, domain.StarsTransactionQuery{})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("missing transaction source = %v", err)
	}
}

func TestWalletCreationReplayDoesNotAcceptImportChallenge(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(filepath.Join(dir, "wallet"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	keys, err := LoadOrCreateKeyCipher(filepath.Join(dir, "keys.key"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil, "wallet.test", WithBalanceSource(&stubBalance{}), WithKeyCipher(keys))
	if _, _, err = bindImportedWallet(t, service); err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.Create(context.Background(), 42, [8]byte{1}, false); !errors.Is(err, domain.ErrWalletProofInvalid) {
		t.Fatalf("import challenge replay as create = %v", err)
	}
}
func TestWalletCustodyKeyRestartAndImportReplacement(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(filepath.Join(dir, "wallet"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	keys, err := LoadOrCreateKeyCipher(filepath.Join(dir, "keys.key"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil, "wallet.test", WithBalanceSource(&stubBalance{}), WithKeyCipher(keys))
	ctx := context.Background()
	key := [8]byte{1}
	if _, err = service.GetProofChallenge(ctx, 42, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.Create(ctx, 42, key, false); err != nil {
		t.Fatal(err)
	}
	if err = store.ValidateKeyCipher(keys); err != nil {
		t.Fatal(err)
	}
	other, err := newKeyCipher(bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ValidateKeyCipher(other); !errors.Is(err, ErrWalletKeyFile) {
		t.Fatalf("wrong master key = %v", err)
	}
	c, err := service.GetProofChallenge(ctx, 42, key)
	if err != nil {
		t.Fatal(err)
	}
	imported := domain.WalletLink{UserID: 42, Address: "imported", PublicKey: bytes.Repeat([]byte{9}, 32)}
	if err = store.CommitWalletLink(ctx, c, imported, [32]byte{1}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	record, err := store.read(42)
	if err != nil {
		t.Fatal(err)
	}
	if record.Key != nil {
		t.Fatal("import kept an unrelated custody key")
	}
}
