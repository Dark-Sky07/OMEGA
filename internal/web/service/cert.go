package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// CertRenewBeforeDays is the window inside which the daily auto-renewal job
// refreshes a Let's Encrypt certificate before it expires.
const CertRenewBeforeDays = 30

// CertService issues, inspects, and auto-renews TLS certificates for the
// panel web server and Xray inbounds.
type CertService struct {
	settingService SettingService
	// DirectoryURL overrides acme.LetsEncryptURL in tests.
	DirectoryURL string
	// HTTP01Addr overrides ":80" in tests so non-root unit tests can run a
	// complete RFC 8555 HTTP-01 flow against an in-process fake ACME server.
	HTTP01Addr string
}

// CertStatus describes the currently configured panel certificate.
type CertStatus struct {
	Configured bool     `json:"configured" example:"true"`
	CertFile   string   `json:"certFile" example:"/etc/x-ui/cert/example.com/fullchain.pem"`
	KeyFile    string   `json:"keyFile" example:"/etc/x-ui/cert/example.com/privkey.pem"`
	Subject    string   `json:"subject,omitempty" example:"example.com"`
	DNSNames   []string `json:"dnsNames,omitempty"`
	Issuer     string   `json:"issuer,omitempty" example:"R11"`
	SelfSigned bool     `json:"selfSigned" example:"false"`
	NotBefore  int64    `json:"notBefore,omitempty" example:"1736000000000"`
	NotAfter   int64    `json:"notAfter,omitempty" example:"1743776000000"`
	DaysLeft   int      `json:"daysLeft" example:"62"`
	Expired    bool     `json:"expired" example:"false"`
	Error      string   `json:"error,omitempty"`
}

// CertDir returns the directory under which managed certificates are stored.
func CertDir() string {
	return filepath.Join(config.GetDBFolderPath(), "cert")
}

// ValidateCertDomain checks that domain is a clean single hostname or IP
// suitable for certificate issuance (no path separators, spaces, or schemes).
func ValidateCertDomain(domain string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" {
		return "", common.NewError("domain is required")
	}
	if len(d) > 253 {
		return "", common.NewError("domain is too long")
	}
	if strings.ContainsAny(d, " /\\:@?#\t\r\n") || strings.Contains(d, "..") {
		return "", common.NewError("invalid domain name")
	}
	return d, nil
}

// InspectFile parses the leaf certificate at certFile and returns its status.
func InspectFile(certFile, keyFile string) *CertStatus {
	st := &CertStatus{
		Configured: certFile != "" && keyFile != "",
		CertFile:   certFile,
		KeyFile:    keyFile,
	}
	if certFile == "" {
		return st
	}
	raw, err := os.ReadFile(certFile)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		st.Error = "no PEM certificate found"
		return st
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	now := time.Now()
	st.Subject = cert.Subject.CommonName
	if st.Subject == "" && len(cert.DNSNames) > 0 {
		st.Subject = cert.DNSNames[0]
	}
	st.DNSNames = cert.DNSNames
	st.Issuer = cert.Issuer.CommonName
	if st.Issuer == "" && len(cert.Issuer.Organization) > 0 {
		st.Issuer = cert.Issuer.Organization[0]
	}
	st.SelfSigned = cert.Issuer.String() == cert.Subject.String()
	st.NotBefore = cert.NotBefore.UnixMilli()
	st.NotAfter = cert.NotAfter.UnixMilli()
	st.DaysLeft = int(cert.NotAfter.Sub(now).Hours() / 24)
	st.Expired = now.After(cert.NotAfter)
	return st
}

// CurrentStatus inspects the certificate currently configured on the panel.
func (s *CertService) CurrentStatus() *CertStatus {
	certFile, _ := s.settingService.GetCertFile()
	keyFile, _ := s.settingService.GetKeyFile()
	return InspectFile(certFile, keyFile)
}

// IssueSelfSigned generates a 365-day ECDSA P-256 self-signed certificate for
// domain (hostname or IP), writes fullchain.pem + privkey.pem under baseDir,
// and optionally applies the paths to the panel settings.
func (s *CertService) IssueSelfSigned(domain string, baseDir string, applyToPanel bool) (*CertStatus, error) {
	clean, err := ValidateCertDomain(domain)
	if err != nil {
		return nil, err
	}
	if baseDir == "" {
		baseDir = CertDir()
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: clean, Organization: []string{"PHOENIX Self-Signed"}},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(clean); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
	} else {
		tpl.DNSNames = []string{clean}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}
	certFile, keyFile, err := writeCertAndKey(filepath.Join(baseDir, clean), [][]byte{der}, priv)
	if err != nil {
		return nil, err
	}
	if applyToPanel {
		_ = s.settingService.SetCertFile(certFile)
		_ = s.settingService.SetKeyFile(keyFile)
	}
	return InspectFile(certFile, keyFile), nil
}

// IssueLetsEncrypt obtains a publicly-trusted certificate for domain via RFC
// 8555 HTTP-01 challenge on port 80, writes fullchain.pem + privkey.pem under
// baseDir, and optionally applies the paths to the panel settings.
func (s *CertService) IssueLetsEncrypt(ctx context.Context, domain string, email string, baseDir string, applyToPanel bool) (*CertStatus, error) {
	clean, err := ValidateCertDomain(domain)
	if err != nil {
		return nil, err
	}
	if net.ParseIP(clean) != nil {
		return nil, common.NewError("Let's Encrypt HTTP-01 requires a domain name (use self-signed for a bare IP)")
	}
	if baseDir == "" {
		baseDir = CertDir()
	}
	acctKey, err := loadOrCreateAccountKey(baseDir)
	if err != nil {
		return nil, fmt.Errorf("acme account key: %w", err)
	}
	dirURL := s.DirectoryURL
	if dirURL == "" {
		dirURL = acme.LetsEncryptURL
	}
	client := &acme.Client{
		Key:          acctKey,
		DirectoryURL: dirURL,
	}
	var contact []string
	if e := strings.TrimSpace(email); e != "" {
		contact = []string{"mailto:" + e}
	}
	if _, err := client.Register(ctx, &acme.Account{Contact: contact}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("acme register: %w", err)
	}

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(clean))
	if err != nil {
		return nil, fmt.Errorf("acme order: %w", err)
	}
	for _, authURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authURL)
		if err != nil {
			return nil, fmt.Errorf("acme authz: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		var chal *acme.Challenge
		for _, c := range authz.Challenges {
			if c.Type == "http-01" {
				chal = c
				break
			}
		}
		if chal == nil {
			return nil, errors.New("no http-01 challenge offered by CA")
		}
		respBody, err := client.HTTP01ChallengeResponse(chal.Token)
		if err != nil {
			return nil, err
		}
		chalPath := client.HTTP01ChallengePath(chal.Token)
		stopHTTP, err := s.startHTTP01Server(chalPath, respBody)
		if err != nil {
			return nil, fmt.Errorf("cannot bind HTTP-01 listener (is port 80 free?): %w", err)
		}
		_, acceptErr := client.Accept(ctx, chal)
		if acceptErr == nil {
			_, acceptErr = client.WaitAuthorization(ctx, authz.URI)
		}
		stopHTTP()
		if acceptErr != nil {
			return nil, fmt.Errorf("http-01 verification failed for %s: %w", clean, acceptErr)
		}
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: clean},
		DNSNames: []string{clean},
	}, leafKey)
	if err != nil {
		return nil, err
	}
	order, err = client.WaitOrder(ctx, order.URI)
	if err != nil {
		return nil, fmt.Errorf("acme wait order: %w", err)
	}
	derChain, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, fmt.Errorf("acme finalize: %w", err)
	}
	certFile, keyFile, err := writeCertAndKey(filepath.Join(baseDir, clean), derChain, leafKey)
	if err != nil {
		return nil, err
	}
	if applyToPanel {
		_ = s.settingService.SetCertFile(certFile)
		_ = s.settingService.SetKeyFile(keyFile)
	}
	return InspectFile(certFile, keyFile), nil
}

// RenewIfDue inspects the panel's active certificate and automatically renews
// it via Let's Encrypt when it is not self-signed and has fewer than
// CertRenewBeforeDays remaining. Safe to call daily from cron.
func (s *CertService) RenewIfDue(ctx context.Context) (bool, error) {
	st := s.CurrentStatus()
	if !st.Configured || st.Error != "" || st.SelfSigned || st.Subject == "" {
		return false, nil
	}
	if st.DaysLeft > CertRenewBeforeDays {
		return false, nil
	}
	logger.Infof("Auto-renewing TLS certificate for %s (%d days remaining)", st.Subject, st.DaysLeft)
	_, err := s.IssueLetsEncrypt(ctx, st.Subject, "", "", true)
	if err != nil {
		logger.Warningf("Auto-renewal for %s failed: %v", st.Subject, err)
		return false, err
	}
	logger.Infof("TLS certificate for %s renewed successfully", st.Subject)
	return true, nil
}

func (s *CertService) startHTTP01Server(chalPath, respBody string) (func(), error) {
	addr := s.HTTP01Addr
	if addr == "" {
		addr = ":80"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc(chalPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(respBody))
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}, nil
}

func loadOrCreateAccountKey(baseDir string) (*ecdsa.PrivateKey, error) {
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(baseDir, "acme-account.key")
	if raw, err := os.ReadFile(keyPath); err == nil {
		if block, _ := pem.Decode(raw); block != nil {
			if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
				return k, nil
			}
		}
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(keyPath, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

func writeCertAndKey(domainDir string, derChain [][]byte, key *ecdsa.PrivateKey) (string, string, error) {
	if err := os.MkdirAll(domainDir, 0o700); err != nil {
		return "", "", err
	}
	var certPEM []byte
	for _, der := range derChain {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certFile := filepath.Join(domainDir, "fullchain.pem")
	keyFile := filepath.Join(domainDir, "privkey.pem")
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}
