package controller

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

// CertificateProblem says what is wrong with text that is meant to be one or
// more PEM certificates, or returns "" when there is nothing wrong. Nothing at
// all is fine: an empty box means the system's trust store.
//
// It is written for a person looking at the box the text is in. A private key is
// named as one, because that is the mistake a cluster invites (its authority
// keeps the private half beside the public one), and the text itself is never
// repeated, so a key pasted by mistake is not echoed into a response or a log.
func CertificateProblem(text string) string {
	rest := []byte(strings.TrimSpace(text))
	if len(rest) == 0 {
		return ""
	}
	found := 0
	for {
		block, after := pem.Decode(rest)
		if block == nil {
			break
		}
		switch {
		case strings.Contains(block.Type, "PRIVATE KEY"):
			return "that is a private key, not a certificate. Paste the certificate, which begins -----BEGIN CERTIFICATE-----; " +
				"a key must not leave the machine it belongs to."
		case block.Type != "CERTIFICATE":
			return fmt.Sprintf("that holds a %s block, and only certificates belong here. Paste what begins -----BEGIN CERTIFICATE-----.", block.Type)
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return fmt.Sprintf("a certificate in it could not be read (%v). Paste it again, whole, from -----BEGIN CERTIFICATE----- to -----END CERTIFICATE-----.", err)
		}
		found++
		rest = after
	}
	switch {
	// What pem.Decode could not read as a block is only text between and around
	// them unless it still carries a header, which is a block that did not parse.
	case bytes.Contains(rest, []byte("-----BEGIN")):
		return "part of it is a PEM block that is cut short or damaged. Paste it again, whole, from -----BEGIN CERTIFICATE----- to -----END CERTIFICATE-----."
	case found == 0:
		return "that holds no PEM certificate. It must begin with -----BEGIN CERTIFICATE-----."
	}
	return ""
}

// certificateAuthority is the certificate text a provider trusts, or nothing
// when what is stored is not entirely certificates.
//
// It is returned so that an edit form can keep it: a form cannot keep what it was
// never shown, and a box that opens empty reads as "nothing is trusted". That is
// safe only because it is a certificate, the public half of a cluster's own
// authority. The field is free text that was not always checked on the way in,
// and a cluster keeps the private half in the same directory as the public one,
// so a key pasted by mistake can already be stored. It is withheld here rather
// than sent to every response, event and audit row that renders a provider, and
// it is the same check that decides what is copied into a machine.
func certificateAuthority(text string) string {
	if strings.TrimSpace(text) == "" || CertificateProblem(text) != "" {
		return ""
	}
	return text
}
