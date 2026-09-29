package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const passwordIterations = 210_000

type Vault struct {
	aead cipher.AEAD
}

func NewVault(dataDir, configuredKey string) (*Vault, error) {
	keyMaterial := []byte(configuredKey)
	if len(keyMaterial) == 0 {
		path := filepath.Join(dataDir, "master.key")
		stored, err := os.ReadFile(path)
		switch {
		case err == nil:
			keyMaterial = []byte(strings.TrimSpace(string(stored)))
		case errors.Is(err, os.ErrNotExist):
			raw := make([]byte, 32)
			if _, err := io.ReadFull(rand.Reader, raw); err != nil {
				return nil, fmt.Errorf("generate master key: %w", err)
			}
			keyMaterial = []byte(base64.RawURLEncoding.EncodeToString(raw))
			if err := os.WriteFile(path, append(keyMaterial, '\n'), 0o600); err != nil {
				return nil, fmt.Errorf("persist master key: %w", err)
			}
		default:
			return nil, fmt.Errorf("read master key: %w", err)
		}
	}

	digest := sha256.Sum256(keyMaterial)
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func (v *Vault) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := v.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return "v1." + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (v *Vault) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, "v1.") {
		return "", errors.New("unsupported ciphertext version")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, "v1."))
	if err != nil {
		return "", errors.New("invalid ciphertext")
	}
	if len(payload) < v.aead.NonceSize() {
		return "", errors.New("invalid ciphertext length")
	}
	nonce, sealed := payload[:v.aead.NonceSize()], payload[v.aead.NonceSize():]
	plaintext, err := v.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", errors.New("decrypt secret: authentication failed")
	}
	return string(plaintext), nil
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	digest := pbkdf2SHA256([]byte(password), salt, passwordIterations, 32)
	return "pbkdf2_sha256$" + strconv.Itoa(passwordIterations) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(digest), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10_000 || iterations > 2_000_000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func RandomToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	const hashLen = 32
	blocks := (keyLen + hashLen - 1) / hashLen
	derived := make([]byte, 0, blocks*hashLen)
	for blockIndex := 1; blockIndex <= blocks; blockIndex++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(blockIndex >> 24), byte(blockIndex >> 16), byte(blockIndex >> 8), byte(blockIndex)})
		u := mac.Sum(nil)
		result := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range result {
				result[j] ^= u[j]
			}
		}
		derived = append(derived, result...)
	}
	return derived[:keyLen]
}
