// Package certs loads or generates the self-signed certificate used for the
// tunnel TLS listener. Ported from Utility.GetOrCreateCertificate; the on-disk
// format is PEM (cert.pem + key.pem) rather than the .NET .pfx.
package certs

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
	"time"
)

// GetOrCreate loads the certificate/key pair at certPath/keyPath, generating a
// new self-signed RSA-2048 cert (CN=localhost, 5-year validity) and writing it to
// disk when either file is missing.
func GetOrCreate(certPath, keyPath string) (tls.Certificate, error) {
	if fileExists(certPath) && fileExists(keyPath) {
		return tls.LoadX509KeyPair(certPath, keyPath)
	}
	return generate(certPath, keyPath)
}

func generate(certPath, keyPath string) (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("serial: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             now,
		NotAfter:              now.AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(key)})

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write key: %w", err)
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

func mustPKCS8(key *rsa.PrivateKey) []byte {
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		// RSA keys always marshal; panic keeps the signature clean.
		panic(err)
	}
	return b
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
