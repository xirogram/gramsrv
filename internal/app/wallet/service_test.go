package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"telesrv/internal/domain"
)

type stubBalance struct {
	balance int64
	calls   int
}

func (s *stubBalance) AccountBalance(context.Context, int64) (int64, error) {
	s.calls++
	return s.balance, nil
}

// A wallet without a TON provider is a supported configuration: telesrv does
// not contact any TON node by default. Constructing it must not panic, and
// provider-backed calls must fail closed instead of pretending to work.
func TestServiceWithoutProviderFailsClosed(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "wallet"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := NewService(store, nil, "wallet.test")
	if _, err := service.Perform(context.Background(), "/api/v2/getAddressBalance", nil, nil); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("Perform without provider = %v, want ErrProviderUnavailable", err)
	}
	if _, err := service.Balance(context.Background(), "EQ-test"); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("Balance without provider = %v, want ErrProviderUnavailable", err)
	}
	// A nil balance source must not silently degrade into a stored zero.
	if _, _, err := bindImportedWallet(t, service); err == nil {
		t.Fatal("ReplaceImported succeeded without any balance source")
	}
}

// With the local TON ledger wired, binding reports the ledger balance instead of
// contacting a provider.
func TestServiceUsesLocalLedgerBalance(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "wallet"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ledger := &stubBalance{balance: 12_345_678}
	service := NewService(store, nil, "wallet.test", WithBalanceSource(ledger))
	link, balance, err := bindImportedWallet(t, service)
	if err != nil {
		t.Fatal(err)
	}
	if balance != ledger.balance || link.Address == "" || len(link.PublicKey) != ed25519.PublicKeySize {
		t.Fatalf("link=%+v balance=%d", link, balance)
	}
	if ledger.calls != 1 {
		t.Fatalf("ledger lookups = %d, want 1", ledger.calls)
	}
	if _, _, err := service.ReplaceImported(context.Background(), 42, [8]byte{1}, link.PublicKey, 1, bytes.Repeat([]byte{1}, 64), false); err == nil {
		t.Fatal("expired challenge was accepted")
	}
}

func TestServiceRejectsMissingDependency(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewService accepted a nil store")
		}
	}()
	NewService(nil, nil, "wallet.test")
}

// Server-held creation must produce a real mainnet address, keep the private key
// only as ciphertext, and stay idempotent across a client retry.
func TestServiceCreatesServerHeldWallet(t *testing.T) {
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
	service := NewService(store, nil, "wallet.test",
		WithBalanceSource(TONLedgerBalanceSource(func(context.Context, int64) (int64, error) { return 777, nil })),
		WithKeyCipher(keys))

	ctx := context.Background()
	if _, err = service.GetProofChallenge(ctx, 42, [8]byte{1}); err != nil {
		t.Fatal(err)
	}
	link, balance, err := service.Create(ctx, 42, [8]byte{1}, false)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 777 || len(link.PublicKey) != ed25519.PublicKeySize {
		t.Fatalf("link=%+v balance=%d", link, balance)
	}
	addr, err := InitialAddress(link.PublicKey, false)
	if err != nil {
		t.Fatal(err)
	}
	if link.Address != addr.String() {
		t.Fatalf("address %s is not the wallet revision-00 address %s", link.Address, addr.String())
	}
	// The plaintext key must exist nowhere in the persisted record.
	raw, err := os.ReadFile(filepath.Join(dir, "wallet", "42.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("private")) || bytes.Contains(raw, link.PublicKey[:16]) {
		t.Fatalf("persisted record leaks key material: %s", raw)
	}
	stored, err := store.WalletLinks(ctx, []int64{42})
	if err != nil || len(stored) != 1 || stored[0].Address != link.Address {
		t.Fatalf("stored links = %+v err = %v", stored, err)
	}
	// A retry inside the same challenge window must not mint a second wallet.
	retry, _, err := service.Create(ctx, 42, [8]byte{1}, false)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Address != link.Address {
		t.Fatalf("retry created %s instead of reusing %s", retry.Address, link.Address)
	}
	again, err := store.WalletLinks(ctx, []int64{42})
	if err != nil || len(again) != 1 {
		t.Fatalf("retry changed stored links: %+v err = %v", again, err)
	}
}

// The sealed key must open again with the same master key and fail under another.
func TestKeyCipherRoundTripAndIsolation(t *testing.T) {
	dir := t.TempDir()
	keys, err := LoadOrCreateKeyCipher(filepath.Join(dir, "keys.key"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := LoadOrCreateKeyCipher(filepath.Join(dir, "other.key"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext := bytes.Repeat([]byte{0xAB}, ed25519.PrivateKeySize)
	ciphertext, nonce, err := keys.Seal(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := keys.Open(ciphertext, nonce)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("round trip = %x err = %v", opened, err)
	}
	if _, err = other.Open(ciphertext, nonce); err == nil {
		t.Fatal("a different master key opened the wallet key")
	}
	if _, err = keys.Open(ciphertext, append(nonce, 0)); err == nil {
		t.Fatal("a tampered nonce opened the wallet key")
	}
	info, err := os.Stat(filepath.Join(dir, "keys.key"))
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("master key file permissions = %v err = %v", info.Mode(), err)
	}
	if err = os.Chmod(filepath.Join(dir, "keys.key"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadOrCreateKeyCipher(filepath.Join(dir, "keys.key")); err == nil {
		t.Fatal("a world-readable master key file was accepted")
	}
}

// Without a master key telesrv must refuse to generate a wallet rather than
// store an unsealed private key.
func TestCreateRequiresKeyCipher(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "wallet"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := NewService(store, nil, "wallet.test",
		WithBalanceSource(TONLedgerBalanceSource(func(context.Context, int64) (int64, error) { return 0, nil })))
	ctx := context.Background()
	if _, err = service.GetProofChallenge(ctx, 42, [8]byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.Create(ctx, 42, [8]byte{1}, false); !errors.Is(err, ErrWalletCreationDisabled) {
		t.Fatalf("Create without key cipher = %v, want ErrWalletCreationDisabled", err)
	}
	links, err := store.WalletLinks(ctx, []int64{42})
	if err != nil || len(links) != 0 {
		t.Fatalf("refused creation left state behind: %+v err = %v", links, err)
	}
}

// bindImportedWallet walks the documented ownership-proof exchange and returns
// the committed link plus the balance the service resolved for it.
func bindImportedWallet(t *testing.T, service *Service) (domain.WalletLink, int64, error) {
	t.Helper()
	ctx := context.Background()
	challenge, err := service.GetProofChallenge(ctx, 42, [8]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, 32))
	public := []byte(private.Public().(ed25519.PublicKey))
	address, err := InitialAddress(public, false)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := time.Now().Unix()
	hash, err := ProofHash(0, address.Data(), challenge.Domain, uint64(timestamp), challenge.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return service.ReplaceImported(ctx, 42, [8]byte{1}, public, int(timestamp), ed25519.Sign(private, hash[:]), false)
}
