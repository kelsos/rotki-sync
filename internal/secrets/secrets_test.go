package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(
		filepath.Join(dir, "secrets.age"),
		"rotki-sync-test",
		"age-identity",
		filepath.Join(dir, "identity.key"),
	)
}

func TestInitSetGetRoundtrip(t *testing.T) {
	keyring.MockInit()
	st := newStore(t)

	rec, mode, err := st.Init()
	if err != nil {
		t.Fatal(err)
	}
	if mode != "keyring" {
		t.Fatalf("mode = %q, want keyring", mode)
	}
	if rec == "" || !st.Exists() {
		t.Fatalf("expected recipient and store file, rec=%q exists=%v", rec, st.Exists())
	}

	if err := st.Set(ScopeUsers, "alice", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(ScopeUsers, "bob", "hunter2"); err != nil {
		t.Fatal(err)
	}

	v, ok, err := st.Get(ScopeUsers, "alice")
	if err != nil || !ok || v != "s3cret" {
		t.Fatalf("Get(alice) = %q ok=%v err=%v", v, ok, err)
	}

	keys, err := st.Keys(ScopeUsers)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "alice" || keys[1] != "bob" {
		t.Fatalf("Keys = %v, want [alice bob]", keys)
	}

	if err := st.Rm(ScopeUsers, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.Get(ScopeUsers, "alice"); ok {
		t.Fatal("alice should be gone after Rm")
	}
}

// When the keyring is unavailable, Init must fall back to a 0600 key file and the
// store must still round-trip via that file.
func TestKeyfileFallbackWhenKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keyring"))
	st := newStore(t)

	_, mode, err := st.Init()
	if err != nil {
		t.Fatal(err)
	}
	if mode != "file" {
		t.Fatalf("mode = %q, want file", mode)
	}

	info, err := os.Stat(st.keyFile)
	if err != nil {
		t.Fatalf("expected key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file perm = %o, want 600", perm)
	}

	if err := st.Set(ScopeUsers, "carol", "pw"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := st.Get(ScopeUsers, "carol"); !ok || v != "pw" {
		t.Fatalf("Get(carol) = %q ok=%v", v, ok)
	}
}

// The env override identity must take precedence over the keyring.
func TestEnvOverrideIdentity(t *testing.T) {
	keyring.MockInit()
	st := newStore(t)
	if _, _, err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(ScopeUsers, "dave", "pw"); err != nil {
		t.Fatal(err)
	}

	// Pull the identity the store just created, expose it via env, then break the
	// keyring: reads must succeed using the env-provided identity.
	id, err := keyring.Get(st.krService, st.krUser)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROTKI_SYNC_AGE_KEY", id)
	keyring.MockInitWithError(errors.New("keyring down"))

	if v, ok, err := st.Get(ScopeUsers, "dave"); err != nil || !ok || v != "pw" {
		t.Fatalf("Get(dave) via env identity = %q ok=%v err=%v", v, ok, err)
	}
}

func TestReadMissingStoreErrors(t *testing.T) {
	keyring.MockInit()
	st := newStore(t)
	if _, err := st.Read(); err == nil {
		t.Fatal("expected error reading a non-existent store")
	}
}
