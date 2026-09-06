package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"golang.org/x/crypto/argon2"
)

// Argon2id bounds simultaneous expensive password work per Admin process.
type Argon2id struct{ slots chan struct{} }

func NewArgon2id() *Argon2id { return &Argon2id{slots: make(chan struct{}, 2)} }
func (h *Argon2id) compute(ctx context.Context, password string, salt []byte, memory, iterations uint32, parallelism uint8) ([]byte, error) {
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, domain.ErrAuthTimeout
	}
	defer func() { <-h.slots }()
	return argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32), nil
}
func (h *Argon2id) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", domain.ErrAuthUnavailable
	}
	hash, err := h.compute(ctx, password, salt, 19*1024, 2, 1)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func (h *Argon2id) Verify(ctx context.Context, password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, domain.ErrAuthUnavailable
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || memory < 19456 || memory > 65536 || iterations < 2 || iterations > 5 || parallelism != 1 {
		return false, domain.ErrAuthUnavailable
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, domain.ErrAuthUnavailable
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false, domain.ErrAuthUnavailable
	}
	actual, err := h.compute(ctx, password, salt, memory, iterations, parallelism)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
