package cryptox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVaultRoundTripAndPersistence(t *testing.T) {
	dir := t.TempDir()
	vault, err := NewVault(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := vault.Encrypt("sk-ant-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "sk-ant-secret" {
		t.Fatal("secret was not encrypted")
	}
	plaintext, err := vault.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "sk-ant-secret" {
		t.Fatalf("unexpected plaintext: %q", plaintext)
	}
	if info, err := os.Stat(filepath.Join(dir, "master.key")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("master key permissions = %v, err = %v", info.Mode().Perm(), err)
	}
	secondVault, err := NewVault(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err = secondVault.Decrypt(ciphertext)
	if err != nil || plaintext != "sk-ant-secret" {
		t.Fatalf("persisted vault could not decrypt: plaintext=%q err=%v", plaintext, err)
	}
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "a sufficiently long password") {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("invalid password accepted")
	}
}
