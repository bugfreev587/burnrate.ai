package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"testing"
)

func TestSplitKey(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKeyID string
		wantSec   string
		wantErr   bool
	}{
		{"valid key", "tg_abc:secretvalue", "tg_abc", "secretvalue", false},
		{"colon at start", ":secret", "", "secret", false},
		{"colon at end", "keyid:", "keyid", "", false},
		{"multiple colons — splits on first", "a:b:c", "a", "b:c", false},
		{"no colon", "nocolonhere", "", "", true},
		{"empty string", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyID, secret, err := splitKey(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("splitKey(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if keyID != tt.wantKeyID {
				t.Errorf("keyID = %q, want %q", keyID, tt.wantKeyID)
			}
			if secret != tt.wantSec {
				t.Errorf("secret = %q, want %q", secret, tt.wantSec)
			}
		})
	}
}

func TestVerifySecret(t *testing.T) {
	pepper := []byte("test-pepper-key")
	svc := &APIKeyService{pepper: pepper}
	salt := []byte("some-salt-value!")
	secret := "my-super-secret"

	// Compute expected hash
	mac := hmac.New(sha256.New, pepper)
	mac.Write(salt)
	mac.Write([]byte(secret))
	correctHash := mac.Sum(nil)

	tests := []struct {
		name       string
		salt       []byte
		storedHash []byte
		secret     string
		want       bool
	}{
		{"correct secret matches", salt, correctHash, secret, true},
		{"wrong secret fails", salt, correctHash, "wrong-secret", false},
		{"wrong salt fails", []byte("wrong-salt-value"), correctHash, secret, false},
		{"wrong hash fails", salt, []byte("not-a-real-hash-at-all-needs-32b!"), secret, false},
		{"empty secret fails", salt, correctHash, "", false},
		{"empty salt fails", nil, correctHash, secret, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.verifySecret(tt.salt, tt.storedHash, tt.secret)
			if got != tt.want {
				t.Errorf("verifySecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrAPIKeyLimitReached(t *testing.T) {
	err := &ErrAPIKeyLimitReached{Limit: 5, Current: 5}
	want := "api key limit reached: 5/5 active keys"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
