package controller

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
)

// certificateAuthority is the certificate text a provider trusts, or nothing
// when what is stored is not entirely certificates.
//
// It is returned so that an edit form can keep it: a form cannot keep what it was
// never shown, and a box that opens empty reads as "nothing is trusted". That is
// safe only because it is a certificate, the public half of a cluster's own
// authority. The field is free text that was never checked on the way in, and a
// cluster keeps the private half in the same directory as the public one, so a
// key pasted by mistake can already be stored. It is withheld here rather than
// sent to every response, event and audit row that renders a provider.
func certificateAuthority(text string) string {
	rest := []byte(text)
	found := false
	for {
		block, after := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return ""
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return ""
		}
		found = true
		rest = after
	}
	// What pem.Decode could not read as a block is only text between and around
	// them unless it still carries a header, which is a block that did not parse.
	if !found || bytes.Contains(rest, []byte("-----BEGIN")) {
		return ""
	}
	return text
}
