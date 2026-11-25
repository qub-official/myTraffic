// Package ca manages a local root certificate authority used to intercept
// HTTPS traffic. The root CA is generated once and stored in the user's
// config directory; leaf certificates for visited hosts are minted on the fly
// and cached in memory. This mirrors how mitmproxy and similar tools work.
package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CA holds the root certificate/key and a cache of generated leaf certs.
type CA struct {
	root    *x509.Certificate
	rootKey *rsa.PrivateKey
	rootPEM []byte

	mu    sync.RWMutex
	cache map[string]*tls.Certificate
}

// Load reads the root CA from dir, creating it on first use.
func Load(dir string) (*CA, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	certPath := filepath.Join(dir, "myTraffic-ca.pem")
	keyPath := filepath.Join(dir, "myTraffic-ca.key")

	certPEM, errC := os.ReadFile(certPath)
	keyPEM, errK := os.ReadFile(keyPath)
	if errC == nil && errK == nil {
		return fromPEM(certPEM, keyPEM)
	}

	// Generate a fresh root CA.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject: pkix.Name{
			CommonName:   "myTraffic Local CA",
			Organization: []string{"myTraffic"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	return fromPEM(certPEM, keyPEM)
}

func fromPEM(certPEM, keyPEM []byte) (*CA, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("ca: invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	kblock, _ := pem.Decode(keyPEM)
	if kblock == nil {
		return nil, fmt.Errorf("ca: invalid key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(kblock.Bytes)
	if err != nil {
		return nil, err
	}
	return &CA{
		root:    cert,
		rootKey: key,
		rootPEM: certPEM,
		cache:   make(map[string]*tls.Certificate),
	}, nil
}

// RootPEM returns the PEM-encoded root certificate for the user to trust.
func (c *CA) RootPEM() []byte { return c.rootPEM }

// Leaf returns a TLS certificate for host, minting and caching one if needed.
func (c *CA) Leaf(host string) (*tls.Certificate, error) {
	c.mu.RLock()
	if cert, ok := c.cache[host]; ok {
		c.mu.RUnlock()
		return cert, nil
	}
	c.mu.RUnlock()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := parseIP(host); ip != nil {
		tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
	} else {
		tmpl.DNSNames = append(tmpl.DNSNames, host)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.root, &key.PublicKey, c.rootKey)
	if err != nil {
		return nil, err
	}
	cert := &tls.Certificate{
		Certificate: [][]byte{der, c.root.Raw},
		PrivateKey:  key,
	}
	c.mu.Lock()
	c.cache[host] = cert
	c.mu.Unlock()
	return cert, nil
}

func serial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	n, _ := rand.Int(rand.Reader, limit)
	if n == nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}
