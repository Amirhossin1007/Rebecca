package node

import "testing"

func TestCertificateHelpers(t *testing.T) {
	cert, key, err := GenerateCertificate("node-test")
	if err != nil {
		t.Fatalf("GenerateCertificate error: %v", err)
	}
	if cert == "" || key == "" {
		t.Fatalf("expected certificate and key")
	}
	publicKey, err := ExtractPublicKeyFromCertificate(cert)
	if err != nil {
		t.Fatalf("ExtractPublicKeyFromCertificate error: %v", err)
	}
	if publicKey == "" {
		t.Fatalf("expected public key")
	}
}
