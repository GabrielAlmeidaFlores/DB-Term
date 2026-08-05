package config

import (
	"os"
	"testing"
)

func TestSecrets_RoundTrip(t *testing.T) {
	usesTempDir(t)

	store, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}

	const connName = "local-postgres"
	const password = "super-secret-password-123!"

	if err := store.SetPassword(connName, password); err != nil {
		t.Fatalf("SetPassword() error: %v", err)
	}

	got, err := store.GetPassword(connName)
	if err != nil {
		t.Fatalf("GetPassword() error: %v", err)
	}
	if got != password {
		t.Errorf("GetPassword() = %q, want %q", got, password)
	}
}

func TestSecrets_PersistAndReload(t *testing.T) {
	usesTempDir(t)

	// First session: set and save.
	store1, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}
	if err := store1.SetPassword("conn-a", "pass-a"); err != nil {
		t.Fatalf("SetPassword() error: %v", err)
	}
	if err := store1.SetPassword("conn-b", "pass-b"); err != nil {
		t.Fatalf("SetPassword() error: %v", err)
	}
	if err := store1.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Second session: reload and retrieve.
	store2, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() (reload) error: %v", err)
	}

	gotA, err := store2.GetPassword("conn-a")
	if err != nil {
		t.Fatalf("GetPassword(conn-a) error: %v", err)
	}
	if gotA != "pass-a" {
		t.Errorf("GetPassword(conn-a) = %q, want %q", gotA, "pass-a")
	}

	gotB, err := store2.GetPassword("conn-b")
	if err != nil {
		t.Fatalf("GetPassword(conn-b) error: %v", err)
	}
	if gotB != "pass-b" {
		t.Errorf("GetPassword(conn-b) = %q, want %q", gotB, "pass-b")
	}
}

func TestSecrets_DeletePassword(t *testing.T) {
	usesTempDir(t)

	store, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}

	if err := store.SetPassword("my-conn", "my-pass"); err != nil {
		t.Fatalf("SetPassword() error: %v", err)
	}
	store.DeletePassword("my-conn")

	_, err = store.GetPassword("my-conn")
	if err == nil {
		t.Error("GetPassword() after DeletePassword() expected error, got nil")
	}
}

func TestSecrets_MissingPassword(t *testing.T) {
	usesTempDir(t)

	store, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}

	_, err = store.GetPassword("does-not-exist")
	if err == nil {
		t.Error("GetPassword() for unknown connection expected error, got nil")
	}
}

func TestSecrets_EmptyStoreOnFirstRun(t *testing.T) {
	usesTempDir(t)

	store, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() on fresh dir error: %v", err)
	}
	if len(store) != 0 {
		t.Errorf("LoadSecrets() on fresh dir: expected empty store, got %d entries", len(store))
	}
}

func TestSecrets_UniqueEncryption(t *testing.T) {
	usesTempDir(t)

	store, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}

	const pass = "same-password"
	if err := store.SetPassword("conn-1", pass); err != nil {
		t.Fatalf("SetPassword(conn-1) error: %v", err)
	}
	if err := store.SetPassword("conn-2", pass); err != nil {
		t.Fatalf("SetPassword(conn-2) error: %v", err)
	}

	// Same plaintext should produce different ciphertexts (different nonces).
	if store["conn-1"] == store["conn-2"] {
		t.Error("SetPassword(): same password produced identical ciphertext (nonce reuse)")
	}
}

func TestSecrets_FilePermissions(t *testing.T) {
	usesTempDir(t)

	store, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}
	if err := store.SetPassword("conn", "pass"); err != nil {
		t.Fatalf("SetPassword() error: %v", err)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	path, err := secretsPath()
	if err != nil {
		t.Fatalf("secretsPath() error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat .secrets error: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf(".secrets permissions = %o, want 0600", perm)
	}
}
