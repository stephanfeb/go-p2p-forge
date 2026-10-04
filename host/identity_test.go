package host

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

// The seed of RFC 8032's first Ed25519 test vector. Every identity format
// below must give the peer ID of this seed.
var testSeed, _ = hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")

func wantID(t *testing.T) peer.ID {
	t.Helper()
	pub := ed25519.NewKeyFromSeed(testSeed).Public().(ed25519.PublicKey)
	priv, err := LoadIdentityFromSeed(testSeed)
	if err != nil {
		t.Fatalf("LoadIdentityFromSeed: %v", err)
	}
	raw, err := priv.GetPublic().Raw()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, pub) {
		t.Fatalf("public key from seed = %x, want %x", raw, pub)
	}
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLoadIdentityFromSeed(t *testing.T) {
	wantID(t)
	if _, err := LoadIdentityFromSeed(testSeed[:31]); err == nil {
		t.Fatal("a 31-byte seed was accepted")
	}
}

func TestLoadIdentityFromFileFormats(t *testing.T) {
	want := wantID(t)
	formats := map[string][]byte{
		"raw seed":    testSeed,
		"hex seed":    []byte(hex.EncodeToString(testSeed) + "\n"),
		"base64 seed": []byte(base64.StdEncoding.EncodeToString(testSeed) + "\n"),
		"64-byte key": ed25519.NewKeyFromSeed(testSeed),
	}
	for name, data := range formats {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "identity.key")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			priv, err := LoadIdentityFromFile(path)
			if err != nil {
				t.Fatalf("LoadIdentityFromFile: %v", err)
			}
			got, err := peer.IDFromPrivateKey(priv)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("peer ID = %s, want %s", got, want)
			}
		})
	}
}

// A key file written by LoadOrCreateIdentity can be passed back to
// LoadIdentityFromFile, so an operator can move a server's identity.
func TestCreatedIdentityLoadsFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peer_identity.key")
	created, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadIdentityFromFile(path)
	if err != nil {
		t.Fatalf("LoadIdentityFromFile: %v", err)
	}
	if !created.Equals(loaded) {
		t.Fatal("loaded key differs from the created one")
	}
}
