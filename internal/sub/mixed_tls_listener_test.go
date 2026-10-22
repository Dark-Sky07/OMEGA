package sub

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestMixedTLSListenerServesHTTPAndSNIHTTPS(t *testing.T) {
	cert := testListenerCertificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mixed := newMixedTLSListener(listener, &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return &cert, nil
		},
	})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })}
	go func() { _ = server.Serve(mixed) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	plain, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("plain HTTP request: %v", err)
	}
	_ = plain.Body.Close()
	if plain.StatusCode != http.StatusOK {
		t.Fatalf("plain HTTP status = %d, want %d", plain.StatusCode, http.StatusOK)
	}

	roots := x509.NewCertPool()
	roots.AddCert(cert.Leaf)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "sub.example.test"}}
	defer transport.CloseIdleConnections()
	secure, err := (&http.Client{Transport: transport}).Get("https://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("SNI HTTPS request: %v", err)
	}
	_ = secure.Body.Close()
	if secure.StatusCode != http.StatusOK {
		t.Fatalf("HTTPS status = %d, want %d", secure.StatusCode, http.StatusOK)
	}
}

func testListenerCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "sub.example.test"},
		DNSNames:              []string{"sub.example.test"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	cert.Leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
