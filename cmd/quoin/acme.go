package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	legodns "github.com/go-acme/lego/v4/providers/dns/route53"
	"github.com/go-acme/lego/v4/registration"

	"github.com/samuel/demy/internal/service"
)

const (
	renewBeforeExpiry = 30 * 24 * time.Hour
	renewCheckEvery   = 12 * time.Hour
)

// legoUser implements registration.User for lego's client.
type legoUser struct {
	email        string
	key          crypto.PrivateKey
	registration *registration.Resource
}

func (u *legoUser) GetEmail() string                        { return u.email }
func (u *legoUser) GetRegistration() *registration.Resource { return u.registration }
func (u *legoUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// dnsCertManager obtains and renews TLS certificates via ACME DNS-01 using Route53.
type dnsCertManager struct {
	domains  []string
	email    string
	cacheDir string
	r53cfg   service.Route53Config
	caURL    string

	mu   sync.RWMutex
	cert *tls.Certificate
}

func newDNSCertManager(domains []string, email, cacheDir string, r53cfg service.Route53Config, staging bool) *dnsCertManager {
	caURL := lego.LEDirectoryProduction
	if staging {
		caURL = lego.LEDirectoryStaging
	}
	return &dnsCertManager{
		domains:  domains,
		email:    email,
		cacheDir: cacheDir,
		r53cfg:   r53cfg,
		caURL:    caURL,
	}
}

// Start loads any existing on-disk certificate, renews if needed, then launches
// the background renewal goroutine. It blocks until the initial certificate is
// available or an error occurs.
func (m *dnsCertManager) Start(ctx context.Context) error {
	if err := os.MkdirAll(m.cacheDir, 0700); err != nil {
		return fmt.Errorf("creating acme cache dir: %w", err)
	}
	if cert, err := m.loadCert(); err == nil {
		m.mu.Lock()
		m.cert = cert
		m.mu.Unlock()
	}
	if err := m.renewIfNeeded(ctx); err != nil {
		return err
	}
	go m.renewLoop(ctx)
	return nil
}

// GetCertificate satisfies tls.Config.GetCertificate.
func (m *dnsCertManager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cert == nil {
		return nil, errors.New("acme: no certificate available yet")
	}
	return m.cert, nil
}

func (m *dnsCertManager) renewLoop(ctx context.Context) {
	t := time.NewTicker(renewCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := m.renewIfNeeded(ctx); err != nil {
				slog.Error("certificate renewal failed", "err", err, "domains", m.domains)
			}
		}
	}
}

func (m *dnsCertManager) renewIfNeeded(ctx context.Context) error {
	m.mu.RLock()
	cert := m.cert
	m.mu.RUnlock()
	if cert != nil {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err == nil && time.Until(leaf.NotAfter) > renewBeforeExpiry {
			return nil
		}
	}
	slog.Info("obtaining certificate via DNS-01/Route53", "domains", m.domains)
	return m.obtain(ctx)
}

func (m *dnsCertManager) obtain(_ context.Context) error {
	accountKey, err := m.loadOrCreateAccountKey()
	if err != nil {
		return fmt.Errorf("account key: %w", err)
	}

	user := &legoUser{email: m.email, key: accountKey}

	cfg := lego.NewConfig(user)
	cfg.CADirURL = m.caURL
	cfg.Certificate.KeyType = certcrypto.EC256

	client, err := lego.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("lego client: %w", err)
	}

	provider, err := m.buildRoute53Provider()
	if err != nil {
		return fmt.Errorf("Route53 provider: %w", err)
	}
	if err := client.Challenge.SetDNS01Provider(provider); err != nil {
		return fmt.Errorf("setting DNS-01 provider: %w", err)
	}

	// Register or reuse existing account.
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return fmt.Errorf("ACME registration: %w", err)
	}
	user.registration = reg

	certs, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: m.domains,
		Bundle:  true,
	})
	if err != nil {
		return fmt.Errorf("obtaining certificate: %w", err)
	}

	if err := m.saveCertAndKey(certs.Certificate, certs.PrivateKey); err != nil {
		return fmt.Errorf("saving certificate: %w", err)
	}

	cert, err := m.loadCert()
	if err != nil {
		return fmt.Errorf("loading new certificate: %w", err)
	}
	m.mu.Lock()
	m.cert = cert
	m.mu.Unlock()

	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	slog.Info("certificate obtained", "domains", m.domains, "expires", leaf.NotAfter.Format(time.DateOnly))
	return nil
}

// buildRoute53Provider constructs a lego Route53 DNS provider from the stored config.
// Explicit credentials take precedence; when absent the standard AWS credential
// chain (env vars, ~/.aws/credentials, IAM role) is used.
func (m *dnsCertManager) buildRoute53Provider() (*legodns.DNSProvider, error) {
	r53 := legodns.NewDefaultConfig()

	if m.r53cfg.AccessKeyID != "" {
		r53.AccessKeyID = m.r53cfg.AccessKeyID
	}
	if m.r53cfg.SecretAccessKey != "" {
		r53.SecretAccessKey = m.r53cfg.SecretAccessKey
	}
	if m.r53cfg.Region != "" {
		r53.Region = m.r53cfg.Region
	}
	if m.r53cfg.HostedZoneID != "" {
		r53.HostedZoneID = m.r53cfg.HostedZoneID
	}
	if m.r53cfg.PropagationWait != "" {
		d, err := time.ParseDuration(m.r53cfg.PropagationWait)
		if err != nil {
			return nil, fmt.Errorf("invalid propagation_wait %q: %w", m.r53cfg.PropagationWait, err)
		}
		r53.PropagationTimeout = d
	}

	return legodns.NewDNSProviderConfig(r53)
}

// -- Key and certificate persistence --

func (m *dnsCertManager) accountKeyPath() string {
	return filepath.Join(m.cacheDir, "acme-account.key")
}

func (m *dnsCertManager) certPath() string {
	return filepath.Join(m.cacheDir, m.domains[0]+".crt")
}

func (m *dnsCertManager) keyPath() string {
	return filepath.Join(m.cacheDir, m.domains[0]+".key")
}

func (m *dnsCertManager) loadOrCreateAccountKey() (crypto.Signer, error) {
	data, err := os.ReadFile(m.accountKeyPath())
	if err == nil {
		if block, _ := pem.Decode(data); block != nil {
			if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
				return key, nil
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	pemData := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(m.accountKeyPath(), pemData, 0600); err != nil {
		return nil, fmt.Errorf("saving account key: %w", err)
	}
	return key, nil
}

// saveCertAndKey writes lego's PEM-encoded certificate chain and private key to disk.
func (m *dnsCertManager) saveCertAndKey(certPEM, keyPEM []byte) error {
	if err := os.WriteFile(m.certPath(), certPEM, 0644); err != nil {
		return err
	}
	return os.WriteFile(m.keyPath(), keyPEM, 0600)
}

func (m *dnsCertManager) loadCert() (*tls.Certificate, error) {
	certPEM, err := os.ReadFile(m.certPath())
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(m.keyPath())
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}
