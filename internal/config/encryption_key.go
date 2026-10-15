package config

import (
	"strings"

	"github.com/eyupio/zoomies/internal/cryptox"
)

// KeyOrigin says where a configuration says the instance's encryption key is.
type KeyOrigin int

const (
	// KeyNone: neither the key nor a key file is configured.
	KeyNone KeyOrigin = iota
	// KeyFromValue: the key itself, in security.encryption_key or
	// ZOOMIES_ENCRYPTION_KEY. There is no file, so there is nothing to copy.
	KeyFromValue
	// KeyFromFile: security.encryption_key_file, which is read, and written by
	// the controller on a first run.
	KeyFromFile
)

// EncryptionKeyOrigin is the one statement of which key is in force: the key
// itself wins over the key file, and with neither there is none. For a file it
// also returns the path, with surrounding space removed.
//
// The rule used to be written out by hand in five places (the controller, the
// API server, the backup package twice, and the settings commands), each reading
// the two fields and trimming them. They agreed, and nothing made them. What the
// rule must keep, wherever it is read: a key that is set but unusable is an
// error and never a reason to fall back to the file. A mistyped
// ZOOMIES_ENCRYPTION_KEY that quietly selected an older key file would seal new
// secrets with a key the operator did not choose.
func (c *Config) EncryptionKeyOrigin() (KeyOrigin, string) {
	if strings.TrimSpace(c.Security.EncryptionKey) != "" {
		return KeyFromValue, ""
	}
	if path := strings.TrimSpace(c.Security.EncryptionKeyFile); path != "" {
		return KeyFromFile, path
	}
	return KeyNone, ""
}

// ResolveEncryptionKey reads the key EncryptionKeyOrigin names, and never
// generates one: a restore is never a first run, and a command that only reads
// must not leave a key behind. With none configured it returns cryptox.ErrNoKey;
// a missing key file is an error that wraps os.ErrNotExist, which is how the
// controller tells a first run from a lost key.
func (c *Config) ResolveEncryptionKey() (*cryptox.Key, error) {
	switch origin, path := c.EncryptionKeyOrigin(); origin {
	case KeyFromValue:
		return cryptox.ParseKey(c.Security.EncryptionKey)
	case KeyFromFile:
		return cryptox.LoadKeyFile(path)
	}
	return nil, cryptox.ErrNoKey
}
