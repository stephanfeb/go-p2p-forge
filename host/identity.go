// Package host provides libp2p host creation and identity management.
package host

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// LoadOrCreateIdentity loads an existing Ed25519 private key from disk,
// or generates and persists a new one if none exists.
func LoadOrCreateIdentity(path string) (crypto.PrivKey, error) {
	path = expandPath(path)

	if data, err := os.ReadFile(path); err == nil {
		priv, err := crypto.UnmarshalEd25519PrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshal identity: %w", err)
		}
		return priv, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read identity: %w", err)
	}

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create identity dir: %w", err)
	}

	raw, err := priv.Raw()
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	if err := os.WriteFile(path, raw, 0600); err != nil {
		return nil, fmt.Errorf("save identity: %w", err)
	}

	return priv, nil
}

// LoadIdentityFromSeed creates an Ed25519 private key from a 32-byte seed.
func LoadIdentityFromSeed(seed []byte) (crypto.PrivKey, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	// UnmarshalEd25519PrivateKey takes the 64-byte private key (seed then
	// public key), not the seed.
	return crypto.UnmarshalEd25519PrivateKey(ed25519.NewKeyFromSeed(seed))
}

// LoadIdentityFromFile loads an Ed25519 identity from a file in one of these
// formats: a 32-byte seed as raw bytes, 64 hex characters or 44 base64
// characters; or the 64-byte private key that LoadOrCreateIdentity writes.
func LoadIdentityFromFile(path string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity file: %w", err)
	}

	switch len(data) {
	case ed25519.SeedSize:
		return LoadIdentityFromSeed(data)
	case ed25519.PrivateKeySize:
		return crypto.UnmarshalEd25519PrivateKey(data)
	}

	content := strings.TrimSpace(string(data))

	if len(content) == 64 {
		seed, err := hex.DecodeString(content)
		if err == nil && len(seed) == ed25519.SeedSize {
			return LoadIdentityFromSeed(seed)
		}
	}

	if len(content) == 44 {
		seed, err := base64.StdEncoding.DecodeString(content)
		if err == nil && len(seed) == ed25519.SeedSize {
			return LoadIdentityFromSeed(seed)
		}
	}

	return nil, fmt.Errorf("invalid identity format: expected a 32-byte seed (raw, 64 hex chars or 44 base64 chars) or a 64-byte private key")
}

// PeerIDFromIdentity derives the peer ID from a private key.
func PeerIDFromIdentity(priv crypto.PrivKey) (peer.ID, error) {
	return peer.IDFromPrivateKey(priv)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
