package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// GenerateWireguardKeypair generates a base64 encoded private and public key pair for Wireguard.
func GenerateWireguardKeypair() (privateKey string, publicKey string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", err
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:]), nil
}

// PublicKeyFromPrivate derives a WireGuard public key from a base64-encoded
// 32-byte private key. Keeping this beside GenerateWireguardKeypair makes
// imported/partially-filled native tunnel clients deterministic without ever
// rotating a supplied private key.
func PublicKeyFromPrivate(privateKey string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return "", fmt.Errorf("wireguard private key is not base64: %w", err)
	}
	if len(decoded) != 32 {
		return "", fmt.Errorf("wireguard private key has %d bytes, want 32", len(decoded))
	}
	var priv, pub [32]byte
	copy(priv[:], decoded)
	// Accept a pasted key as-is but apply WireGuard's clamping rules before
	// deriving the public half, matching GenerateWireguardKeypair and the
	// reference implementations.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	curve25519.ScalarBaseMult(&pub, &priv)
	return base64.StdEncoding.EncodeToString(pub[:]), nil
}

// KeyToHex converts a base64-encoded WireGuard/AmneziaWG key into the lower
// case, 32-byte hexadecimal form required by the userspace UAPI.
func KeyToHex(key string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("wireguard key is not base64: %w", err)
	}
	if len(decoded) != 32 {
		return "", fmt.Errorf("wireguard key has %d bytes, want 32", len(decoded))
	}
	return hex.EncodeToString(decoded), nil
}
