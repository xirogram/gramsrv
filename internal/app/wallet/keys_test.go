package wallet

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// The master key file is written hex-encoded, so it has to be decoded on the way
// back in. Reading it as raw bytes made the wallet work on first boot, where the
// freshly generated key was still in hand, and fail on every boot afterwards --
// with a "corrupt key file" error that gave no hint the encoding was the problem.
// A deployment that had been restarted even once could not start its wallet.
func TestLoadOrCreateKeyCipherSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.key")

	first, err := LoadOrCreateKeyCipher(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if first == nil {
		t.Fatal("first load returned no cipher")
	}

	// The file must exist with the documented permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("key file mode = %v, want no group or world bits", info.Mode().Perm())
	}

	// Second load reads the file back off disk -- this is the boot that used to fail.
	second, err := LoadOrCreateKeyCipher(path)
	if err != nil {
		t.Fatalf("reload after restart: %v", err)
	}
	if second == nil {
		t.Fatal("reload returned no cipher")
	}

	// Same key, so data sealed by the first cipher must open with the second.
	plaintext := []byte("address-and-key-material")
	sealed, nonce, err := first.Seal(plaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	opened, err := second.Open(sealed, nonce)
	if err != nil {
		t.Fatalf("open sealed data after restart: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("opened %q, want %q", opened, plaintext)
	}
}

// Whatever is on disk has to come back as 32 bytes, which is the only shape
// newKeyCipher accepts.
func TestDecodeKeyFileRoundTrip(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	for name, onDisk := range map[string][]byte{
		"hex with newline": []byte(hex.EncodeToString(raw) + "\n"),
		"hex no newline":   []byte(hex.EncodeToString(raw)),
	} {
		got := decodeKeyFile("x", onDisk)
		if len(got) != 32 {
			t.Fatalf("%s: decoded to %d bytes, want 32", name, len(got))
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("%s: decoded to the wrong key", name)
		}
	}
}

// A group-readable key file is a real disclosure, so the mode check has to hold
// on reload too, not just at creation.
func TestLoadOrCreateKeyCipherRejectsPermissiveFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.key")
	raw := make([]byte, 32)
	if err := os.WriteFile(path, []byte(hex.EncodeToString(raw)+"\n"), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKeyCipher(path); err == nil {
		t.Fatal("a 0640 master key file was accepted")
	}
}

func TestDecodeKeyFileRejectsUnpublishedRawEncoding(t *testing.T) {
	if decoded := decodeKeyFile("unused", bytes.Repeat([]byte{7}, 32)); decoded != nil {
		t.Fatal("accepted an unsupported raw-key encoding")
	}
}
