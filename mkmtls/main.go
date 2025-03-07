package mkmtls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

const (
	caKeyFile  = "ca.key"
	caCertFile = "ca.crt"
)

// createK8sSecretYAML generates a Kubernetes TLS secret YAML from TLS certificate, key and CA
func createK8sSecretYAML(secretName string, keyPEM, certPEM, caPEM []byte) string {
	// Base64 encode the key, certificate and CA
	keyB64 := base64.StdEncoding.EncodeToString(keyPEM)
	certB64 := base64.StdEncoding.EncodeToString(certPEM)
	caB64 := base64.StdEncoding.EncodeToString(caPEM)

	yaml := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
data:
  tls.crt: %s
  tls.key: %s
  ca.crt: %s
`, secretName, certB64, keyB64, caB64)

	return yaml
}

func Main(args []string) {
	// Check if we have any arguments for DNS names
	if len(args) < 2 {
		fmt.Println("Usage: cert-gen [dns_name1] [dns_name2] ...")
		os.Exit(1)
	}

	// Get DNS names from arguments
	dnsNames := args[1:]

	// Check if CA exists, create if not
	caKey, caCert, err := loadOrCreateCA()
	if err != nil {
		fmt.Printf("Error with CA: %v\n", err)
		os.Exit(1)
	}

	// Read CA cert file to get PEM data
	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		fmt.Printf("Error reading CA certificate: %v\n", err)
		os.Exit(1)
	}

	// Generate TLS cert with DNS names
	certName := dnsNames[0] // Use first DNS name as the certificate filename base
	err = generateTLSCert(caKey, caCert, certName, dnsNames, caCertPEM)
	if err != nil {
		fmt.Printf("Error generating certificate: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created TLS certificate for %v\n", dnsNames)
}

func loadOrCreateCA() (ed25519.PrivateKey, *x509.Certificate, error) {
	// Check if CA files exist
	_, keyErr := os.Stat(caKeyFile)
	_, certErr := os.Stat(caCertFile)

	// If both files exist, load them
	if keyErr == nil && certErr == nil {
		fmt.Println("Loading existing CA...")
		return loadCA()
	}

	// Otherwise create a new CA
	fmt.Println("Creating new CA...")
	return createCA()
}

func loadCA() (ed25519.PrivateKey, *x509.Certificate, error) {
	// Read private key
	keyPEM, err := os.ReadFile(caKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA key: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to parse CA key PEM")
	}

	caKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA key: %w", err)
	}

	edKey, ok := caKey.(ed25519.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("private key is not an Ed25519 key")
	}

	// Read certificate
	certPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("failed to parse CA cert PEM")
	}

	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA cert: %w", err)
	}

	return edKey, caCert, nil
}

func createCA() (ed25519.PrivateKey, *x509.Certificate, error) {
	// Generate Ed25519 private key
	_, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	// Marshal the private key to PKCS#8 format
	caKeyBytes, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	// Save private key to file
	caKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: caKeyBytes,
	})
	if err := os.WriteFile(caKeyFile, caKeyPEM, 0o600); err != nil {
		return nil, nil, fmt.Errorf("failed to save CA private key: %w", err)
	}

	// Prepare CA certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"CA for Local Development"},
			CommonName:   "Local Development CA",
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0), // Valid for 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	// Create and sign the CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Save certificate to file
	caCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER,
	})
	if err := os.WriteFile(caCertFile, caCertPEM, 0o644); err != nil {
		return nil, nil, fmt.Errorf("failed to save CA certificate: %w", err)
	}

	// Parse the CA certificate
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse created CA certificate: %w", err)
	}

	return caKey, caCert, nil
}

func generateTLSCert(caKey ed25519.PrivateKey, caCert *x509.Certificate, certName string, dnsNames []string, caCertPEM []byte) error {
	// Generate Ed25519 key for the certificate
	_, certKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate certificate key: %w", err)
	}

	// Prepare certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	certTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Local Development"},
			CommonName:   dnsNames[0], // Use first DNS name as CN
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")}, // Always include localhost IP
	}

	// Create and sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, caCert, certKey.Public(), caKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Marshal the private key to PKCS#8 format
	certKeyBytes, err := x509.MarshalPKCS8PrivateKey(certKey)
	if err != nil {
		return fmt.Errorf("failed to marshal certificate key: %w", err)
	}

	// Save certificate key to file
	keyFileName := certName + ".key"
	certKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: certKeyBytes,
	})
	if err := os.WriteFile(keyFileName, certKeyPEM, 0o600); err != nil {
		return fmt.Errorf("failed to save certificate key: %w", err)
	}

	// Save certificate to file
	certFileName := certName + ".crt"
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	if err := os.WriteFile(certFileName, certPEM, 0o644); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	// Create Kubernetes Secret YAML
	secretName := strings.ReplaceAll(certName, ".", "-") + "-mtls"
	secretYAML := createK8sSecretYAML(secretName, certKeyPEM, certPEM, caCertPEM)
	secretFileName := certName + "-tls-secret.yaml"

	if err := os.WriteFile(secretFileName, []byte(secretYAML), 0o644); err != nil {
		return fmt.Errorf("failed to save Kubernetes secret YAML: %w", err)
	}

	fmt.Printf("Created certificate: %s and key: %s\n", certFileName, keyFileName)
	fmt.Printf("Created Kubernetes TLS secret YAML: %s\n", secretFileName)
	return nil
}
