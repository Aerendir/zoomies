package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/eyupio/zoomies/internal/releasekey"
)

// runRelease is the release signing key: minting it once, signing a release's
// checksums with it in the pipeline, and verifying a download against it by
// hand. The running binary trusts the key set compiled into it, so this is the
// other half of the same mechanism (internal/releasekey).
func runRelease(ctx context.Context, e *env, args []string) error {
	subs := []*subcommand{
		{"keygen", "--out DIR", "Mint the release signing key pair", runReleaseKeygen},
		{"sign", "[checksums]", "Write the detached signature for a release's checksums", runReleaseSign},
		{"verify", "checksums sig", "Check a release's signature against a key", runReleaseVerify},
	}
	return runGroup(ctx, e, "release", "The release signing key a download is held to (docs/upgrading.md).", subs, args)
}

// runReleaseKeygen mints a pair and writes both halves to a directory. The
// private half is a credential: it is stored as a release secret and never
// committed; the public half is committed so every binary can trust it.
func runReleaseKeygen(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "zoomies release keygen --out DIR", "Mint the release signing key pair.")
	out := fs.String("out", "", "directory to write release.pub and release.key into")
	fs.example("zoomies release keygen --out ./release-key")
	if err := fs.parse(args); err != nil {
		return err
	}
	if err := fs.noMoreArgs(); err != nil {
		return err
	}
	if strings.TrimSpace(*out) == "" {
		return usagef("release keygen", "pass --out DIR: the private key is written to a file and is never printed")
	}
	public, private, err := releasekey.Generate()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	pubPath := filepath.Join(*out, "release.pub")
	keyPath := filepath.Join(*out, "release.key")
	if err := os.WriteFile(pubPath, []byte(public+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, []byte(private+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(e.out, "Wrote %s (public, commit it) and %s (private, mode 0600).\n", pubPath, keyPath)
	fmt.Fprintf(e.out, "Store %s as the repository secret RELEASE_SIGNING_KEY and delete it from this machine; docs/upgrading.md, signing releases.\n", keyPath)
	return nil
}

// runReleaseSign writes the detached signature a release carries beside its
// checksums.txt. It is what the release workflow runs, and what an owner runs
// to check the mechanism by hand.
func runReleaseSign(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "zoomies release sign --key-file FILE [checksums]", "Write the detached signature for a release's checksums.")
	keyFile := fs.String("key-file", "", "a file holding the base64 private key; - reads it from standard input")
	key := fs.String("key", "", "the base64 private key itself; prefer --key-file, since a flag's value is in the process list")
	fs.example("zoomies release sign --key-file release.key dist/checksums.txt > dist/checksums.txt.ed25519")
	if err := fs.parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return usagef("release sign", "give at most the checksums file, but %d arguments were given", fs.NArg())
	}
	private := strings.TrimSpace(*key)
	if private == "" {
		var err error
		if private, err = readReleaseKey(e, *keyFile); err != nil {
			return err
		}
	}
	var (
		msg []byte
		err error
	)
	if fs.NArg() == 0 {
		msg, err = io.ReadAll(e.in)
	} else {
		msg, err = os.ReadFile(fs.Arg(0))
	}
	if err != nil {
		return err
	}
	envelope, err := releasekey.Sign(private, msg)
	if err != nil {
		return err
	}
	_, err = e.out.Write(envelope)
	return err
}

// runReleaseVerify holds a checksums file and its signature to a key: the one
// named by --key or --key-file, or the set compiled into this binary.
func runReleaseVerify(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "zoomies release verify [--key KEY | --key-file FILE] checksums sig",
		"Check a release's signature against a key.")
	key := fs.String("key", "", "the base64 public key to check against; empty uses the key compiled into this binary")
	keyFile := fs.String("key-file", "", "a file holding the base64 public key to check against")
	fs.example("zoomies release verify --key-file release.pub dist/checksums.txt dist/checksums.txt.ed25519")
	if err := fs.parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usagef("release verify", "give the checksums file and its signature")
	}
	set := strings.TrimSpace(*key)
	if set == "" && strings.TrimSpace(*keyFile) != "" {
		var err error
		if set, err = readReleaseKey(e, *keyFile); err != nil {
			return err
		}
	}
	if set == "" {
		// No key named: the set compiled into this binary, which is what an
		// operator checking a download has in hand.
		set = releasekey.Default
	}
	sums, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return err
	}
	if err := releasekey.Verify(set, sums, sig); err != nil {
		return err
	}
	fmt.Fprintln(e.out, "The signature is valid under the release key.")
	return nil
}

// readReleaseKey reads a base64 key from a file, or from standard input when
// path is "-". An empty path is an error: signing has no default key.
func readReleaseKey(e *env, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", usagef("release", "give --key-file FILE (or --key, for a key that is not a secret)")
	}
	if path == "-" {
		body, err := io.ReadAll(e.in)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(body)), nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
