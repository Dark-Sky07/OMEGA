package openvpn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const certValidity = 10 * 365 * 24 * time.Hour

func caFiles(dir string) (crt, key string) {
	return filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
}

func serverFiles(dir string) (crt, key string) {
	return filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
}

// clientFiles derives per-client cert/key paths from the client email. The
// email is the certificate CN (the identity openvpn reports in CLIENT_LIST);
// the on-disk file name is sanitized so unusual emails stay filesystem-safe.
func clientFiles(dir, email string) (crt, key string) {
	name := sanitizeCertName(email)
	return filepath.Join(dir, "clients", name+".crt"), filepath.Join(dir, "clients", name+".key")
}

func sanitizeCertName(email string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(email) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '_', r == '-', r == '+':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "client"
	}
	return b.String()
}

func newKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func certTemplate(cn string, isCA bool) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	return &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"Omega OpenVPN"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(certValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		IsCA:         isCA,
		BasicConstraintsValid: true,
	}, nil
}

func sign(parent *x509.Certificate, parentKey *ecdsa.PrivateKey, tmpl *x509.Certificate, pub *ecdsa.PublicKey) (*x509.Certificate, error) {
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, parentKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

func writePEM(path string, blockType string, der []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: der})
}

func writeKeyPEM(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

func loadKeyPair(crtPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	crtDer, err := os.ReadFile(crtPath)
	if err != nil {
		return nil, nil, err
	}
	keyDer, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	crtBlock, _ := pem.Decode(crtDer)
	if crtBlock == nil {
		return nil, nil, fmt.Errorf("invalid certificate PEM: %s", crtPath)
	}
	crt, err := x509.ParseCertificate(crtBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode(keyDer)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("invalid key PEM: %s", keyPath)
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return crt, key, nil
}

// ensureCA creates (or loads) the inbound's self-signed CA.
func ensureCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	crtPath, keyPath := caFiles(dir)
	if crt, key, err := loadKeyPair(crtPath, keyPath); err == nil {
		if time.Now().Before(crt.NotAfter.Add(-24 * time.Hour)) {
			return crt, key, nil
		}
	}
	key, err := newKey()
	if err != nil {
		return nil, nil, err
	}
	tmpl, err := certTemplate("Omega OpenVPN CA", true)
	if err != nil {
		return nil, nil, err
	}
	tmpl.KeyUsage |= x509.KeyUsageCertSign
	tmpl.BasicConstraintsValid = true
	self, err := sign(tmpl, key, tmpl, &key.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	if err := writeKeyPEM(keyPath, key); err != nil {
		return nil, nil, err
	}
	der, err := certPEM(self)
	if err != nil {
		return nil, nil, err
	}
	if err := writePEM(crtPath, "CERTIFICATE", der); err != nil {
		return nil, nil, err
	}
	return self, key, nil
}

// ensureServerCert creates (or loads) the server keypair signed by the CA.
func ensureServerCert(dir string) error {
	crtPath, keyPath := serverFiles(dir)
	if _, _, err := loadKeyPair(crtPath, keyPath); err == nil {
		return nil
	}
	caCrt, caKey, err := ensureCA(dir)
	if err != nil {
		return err
	}
	key, err := newKey()
	if err != nil {
		return err
	}
	tmpl, err := certTemplate("openvpn-server", false)
	if err != nil {
		return err
	}
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	signed, err := sign(caCrt, caKey, tmpl, &key.PublicKey)
	if err != nil {
		return err
	}
	if err := writeKeyPEM(keyPath, key); err != nil {
		return err
	}
	der, err := certPEM(signed)
	if err != nil {
		return err
	}
	return writePEM(crtPath, "CERTIFICATE", der)
}

// ensureClientCert creates (or loads) the client keypair (CN = email) signed
// by the CA.
func ensureClientCert(dir, email string) error {
	if email == "" {
		return fmt.Errorf("openvpn: client email is empty")
	}
	if err := os.MkdirAll(filepath.Join(dir, "clients"), 0o750); err != nil {
		return err
	}
	crtPath, keyPath := clientFiles(dir, email)
	if _, _, err := loadKeyPair(crtPath, keyPath); err == nil {
		return nil
	}
	caCrt, caKey, err := ensureCA(dir)
	if err != nil {
		return err
	}
	key, err := newKey()
	if err != nil {
		return err
	}
	tmpl, err := certTemplate(email, false)
	if err != nil {
		return err
	}
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	signed, err := sign(caCrt, caKey, tmpl, &key.PublicKey)
	if err != nil {
		return err
	}
	if err := writeKeyPEM(keyPath, key); err != nil {
		return err
	}
	der, err := certPEM(signed)
	if err != nil {
		return err
	}
	return writePEM(crtPath, "CERTIFICATE", der)
}

// pruneClientCerts removes client certs no longer attached to the inbound.
// `keep` is the current client list; everything else under clients/ goes.
func pruneClientCerts(dir string, keep []string) error {
	keepSet := make(map[string]bool, len(keep))
	for _, email := range keep {
		keepSet[sanitizeCertName(email)] = true
	}
	entries, err := os.ReadDir(filepath.Join(dir, "clients"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if !keepSet[base] {
			_ = os.Remove(filepath.Join(dir, "clients", e.Name()))
		}
	}
	return nil
}

// certPEM returns the certificate's DER bytes. Despite the historical name
// it must NOT be PEM-encoded here: the callers hand the result to writePEM,
// which does the single PEM encoding. Double-encoding produced unreadable
// cert files.
func certPEM(crt *x509.Certificate) ([]byte, error) {
	return crt.Raw, nil
}
