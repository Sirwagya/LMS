package cryptoadapter

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/scrypt"
)

const (
	argonMemory      uint32 = 64 * 1024
	argonIterations  uint32 = 3
	argonParallelism uint8  = 2
	argonSaltLen            = 16
	argonKeyLen      uint32 = 32
)

type DualPasswordHasher struct{}

func NewDualPasswordHasher() *DualPasswordHasher {
	return &DualPasswordHasher{}
}

func (h *DualPasswordHasher) Hash(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(plain), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism, b64Salt, b64Key), nil
}

func (h *DualPasswordHasher) Verify(plain, encoded string) (bool, bool, error) {
	if strings.HasPrefix(encoded, "$argon2id$") {
		ok, err := verifyArgon2id(plain, encoded)
		return ok, false, err
	}
	if strings.Contains(encoded, ":") {
		ok, err := verifyNodeScrypt(plain, encoded)
		return ok, ok, err
	}
	return false, false, errors.New("unsupported password hash format")
}

func verifyArgon2id(plain, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("malformed argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("unsupported argon2id version")
	}
	var mem, iter uint32
	var par uint8
	for _, kv := range strings.Split(parts[3], ",") {
		pair := strings.SplitN(kv, "=", 2)
		if len(pair) != 2 {
			continue
		}
		v, err := strconv.ParseUint(pair[1], 10, 32)
		if err != nil {
			return false, fmt.Errorf("parse argon2id param %s: %w", pair[0], err)
		}
		switch pair[0] {
		case "m":
			mem = uint32(v)
		case "t":
			iter = uint32(v)
		case "p":
			par = uint8(v)
		}
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(plain), salt, iter, mem, par, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// verifyNodeScrypt verifies Node.js `salt + ":" + scryptSync(password, salt, 64).toString("hex")`
// where Node encodes the ASCII hex salt string as UTF-8 bytes.
func verifyNodeScrypt(plain, encoded string) (bool, error) {
	parts := strings.SplitN(encoded, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false, errors.New("malformed scrypt hash")
	}
	expected, err := hex.DecodeString(parts[1])
	if err != nil {
		return false, fmt.Errorf("decode scrypt hex: %w", err)
	}
	actual, err := scrypt.Key([]byte(plain), []byte(parts[0]), 16384, 8, 1, len(expected))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
