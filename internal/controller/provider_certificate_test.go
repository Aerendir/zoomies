package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func testCertificatePEM(t *testing.T, name string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// The certificate a provider trusts is returned so that a form can keep it, and
// that is safe only because it is a certificate. The field was never checked on
// the way in, so a key pasted into it by mistake (a cluster's authority keeps its
// private half beside the public one) can already be stored; it must never leave
// the controller in a response, an event or an audit row.
func TestOnlyACertificateIsEverReturnedAsTheAuthorityAProviderTrusts(t *testing.T) {
	one := testCertificatePEM(t, "pve-root-ca")
	two := testCertificatePEM(t, "pve-leaf")
	key := "-----BEGIN PRIVATE KEY-----\nbm90IGEgcmVhbCBrZXk=\n-----END PRIVATE KEY-----\n"

	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"nothing stored is nothing returned", "", ""},
		{"one certificate comes back as it was stored", one, one},
		{"a bundle of two comes back whole", one + two, one + two},
		{"a note above the certificate does not hide it", "pve-root-ca.pem\n" + one, "pve-root-ca.pem\n" + one},
		{"a private key is never returned", key, ""},
		{"a certificate with a key after it is not returned at all", one + key, ""},
		{"text that is no PEM at all is not returned", "this is not a certificate", ""},
		{"a block that is not a certificate inside is not returned", "-----BEGIN CERTIFICATE-----\nbm90IGEgY2VydGlmaWNhdGU=\n-----END CERTIFICATE-----\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := certificateAuthority(tc.in)
			if got != tc.want {
				t.Errorf("certificateAuthority(%q) = %q, want %q", truncateForTest(tc.in), truncateForTest(got), truncateForTest(tc.want))
			}
			if strings.Contains(got, "PRIVATE KEY") {
				t.Errorf("a private key was returned: %q", truncateForTest(got))
			}
		})
	}
}

func truncateForTest(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}
