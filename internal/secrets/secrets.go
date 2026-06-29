// Package secrets manages an age-encrypted secret store for rotki-sync. The age
// identity (private key) is resolved from, in order, an env override, the OS
// keyring, or a 0600 key file — so it works both interactively (keyring) and
// unattended (key file). Secrets are only ever decrypted into memory and never
// written to disk in plaintext.
package secrets

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
	"github.com/BurntSushi/toml"
	"github.com/zalando/go-keyring"

	"github.com/kelsos/rotki-sync/internal/paths"
)

// ScopeUsers groups per-user login passwords (key = username).
const ScopeUsers = "users"

const (
	// envKeyOverride lets headless/CI runs supply the identity without a keyring.
	envKeyOverride = "ROTKI_SYNC_AGE_KEY"
	keyringService = "rotki-sync"
	keyringUser    = "age-identity"
	storeFileName  = "secrets.age"
	keyFileName    = "identity.key"
)

// Store is a handle to the encrypted secret file plus key-custody settings.
type Store struct {
	path      string
	krService string
	krUser    string
	keyFile   string
	recipient string // public key; empty means derive from the identity
}

// New constructs a Store with explicit paths (used by tests).
func New(path, krService, krUser, keyFile string) *Store {
	return &Store{path: path, krService: krService, krUser: krUser, keyFile: keyFile}
}

// Default returns a Store anchored to rotki-sync's data home (paths.Home()).
func Default() *Store {
	home := paths.Home()
	return New(
		filepath.Join(home, storeFileName),
		keyringService,
		keyringUser,
		filepath.Join(home, keyFileName),
	)
}

// Path returns the encrypted store file path.
func (s *Store) Path() string { return s.path }

// Exists reports whether the encrypted store file is present.
func (s *Store) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// Init generates a fresh age identity, stores the private key in the OS keyring
// (falling back to a 0600 key file when no keyring is available), seeds an empty
// encrypted store, and returns the recipient (public key) plus the key mode
// used ("keyring" or "file").
func (s *Store) Init() (recipient, mode string, err error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return "", "", fmt.Errorf("failed to create data directory: %w", err)
	}

	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", err
	}

	mode = "keyring"
	if kerr := keyring.Set(s.krService, s.krUser, id.String()); kerr != nil {
		if werr := os.WriteFile(s.keyFile, []byte(id.String()+"\n"), 0o600); werr != nil {
			return "", "", fmt.Errorf("keyring unavailable (%v) and key file write failed: %w", kerr, werr)
		}
		mode = "file"
	}

	s.recipient = id.Recipient().String()
	if err := s.write(map[string]map[string]string{}); err != nil {
		return "", "", err
	}
	return s.recipient, mode, nil
}

// identity resolves the age private key: env override, then keyring, then key file.
func (s *Store) identity() (*age.X25519Identity, error) {
	if k := os.Getenv(envKeyOverride); k != "" {
		return age.ParseX25519Identity(strings.TrimSpace(k))
	}
	if k, err := keyring.Get(s.krService, s.krUser); err == nil && k != "" {
		return age.ParseX25519Identity(strings.TrimSpace(k))
	}
	if b, err := os.ReadFile(s.keyFile); err == nil {
		return age.ParseX25519Identity(strings.TrimSpace(string(b)))
	}
	return nil, fmt.Errorf("no age identity found (env %s / keyring / %s); run `rotki-sync secret init`", envKeyOverride, s.keyFile)
}

func (s *Store) recipientObj() (age.Recipient, error) {
	if s.recipient != "" {
		return age.ParseX25519Recipient(s.recipient)
	}
	id, err := s.identity()
	if err != nil {
		return nil, err
	}
	return id.Recipient(), nil
}

// Read decrypts the store into a scope -> key -> value map.
func (s *Store) Read() (map[string]map[string]string, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open secret store (run `rotki-sync secret init`?): %w", err)
	}
	defer func() { _ = f.Close() }()

	id, err := s.identity()
	if err != nil {
		return nil, err
	}
	r, err := age.Decrypt(f, id)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret store: %w", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	out := map[string]map[string]string{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := toml.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("parse decrypted secrets: %w", err)
		}
	}
	return out, nil
}

func (s *Store) write(m map[string]map[string]string) error {
	rec, err := s.recipientObj()
	if err != nil {
		return err
	}

	var plain bytes.Buffer
	if err := toml.NewEncoder(&plain).Encode(m); err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w, err := age.Encrypt(out, rec)
	if err != nil {
		_ = out.Close()
		return err
	}
	if _, err := w.Write(plain.Bytes()); err != nil {
		_ = w.Close()
		_ = out.Close()
		return err
	}
	if err := w.Close(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Get returns a single secret value and whether it was present.
func (s *Store) Get(scope, key string) (string, bool, error) {
	m, err := s.Read()
	if err != nil {
		return "", false, err
	}
	if kv, ok := m[scope]; ok {
		if v, ok := kv[key]; ok {
			return v, true, nil
		}
	}
	return "", false, nil
}

// Set stores a single secret value.
func (s *Store) Set(scope, key, val string) error {
	m, err := s.Read()
	if err != nil {
		return err
	}
	if m[scope] == nil {
		m[scope] = map[string]string{}
	}
	m[scope][key] = val
	return s.write(m)
}

// Rm deletes a single secret value.
func (s *Store) Rm(scope, key string) error {
	m, err := s.Read()
	if err != nil {
		return err
	}
	if m[scope] != nil {
		delete(m[scope], key)
	}
	return s.write(m)
}

// Keys returns the secret key names for a scope (never values), sorted.
func (s *Store) Keys(scope string) ([]string, error) {
	m, err := s.Read()
	if err != nil {
		return nil, err
	}
	ks := make([]string, 0, len(m[scope]))
	for k := range m[scope] {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks, nil
}
