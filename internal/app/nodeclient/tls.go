package nodeclient

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

type TLSConfig struct {
	ClientCertFile string
	ClientKeyFile  string
	ServerCertFile string
	ServerName     string
}

func LoadClientTLS(config TLSConfig) (*tls.Config, error) {
	if config.ClientCertFile == "" || config.ClientKeyFile == "" {
		return nil, fmt.Errorf("client certificate and key are required")
	}
	if config.ServerCertFile == "" {
		return nil, fmt.Errorf("server certificate is required")
	}

	clientCert, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	serverPEM, err := os.ReadFile(config.ServerCertFile)
	if err != nil {
		return nil, fmt.Errorf("read server certificate: %w", err)
	}
	serverRoots := x509.NewCertPool()
	if !serverRoots.AppendCertsFromPEM(serverPEM) {
		return nil, fmt.Errorf("append server certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      serverRoots,
		ServerName:   config.ServerName,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

type PEMTLSConfig struct {
	ClientCertPEM string
	ClientKeyPEM  string
	ServerCertPEM string
	ServerName    string
}

func LoadClientTLSFromPEM(config PEMTLSConfig) (*tls.Config, error) {
	if config.ClientCertPEM == "" || config.ClientKeyPEM == "" {
		return nil, fmt.Errorf("client certificate and key are required")
	}
	if config.ServerCertPEM == "" {
		return nil, fmt.Errorf("server certificate is required")
	}

	clientCert, err := tls.X509KeyPair([]byte(config.ClientCertPEM), []byte(config.ClientKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	serverCert, err := firstCertificate([]byte(config.ServerCertPEM))
	if err != nil {
		return nil, err
	}

	serverName := config.ServerName
	if serverName == "" {
		serverName = serverCert.Subject.CommonName
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		ServerName:         serverName,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("server certificate is missing")
			}
			if !equalBytes(rawCerts[0], serverCert.Raw) {
				return fmt.Errorf("server certificate does not match the pinned node certificate")
			}
			return nil
		},
	}, nil
}

func firstCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("decode server certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse server certificate: %w", err)
	}
	return cert, nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var diff byte
	for i := range left {
		diff |= left[i] ^ right[i]
	}
	return diff == 0
}
