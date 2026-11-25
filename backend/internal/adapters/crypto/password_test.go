package cryptoadapter_test

import (
	"testing"

	cryptoadapter "vedam/lms/internal/adapters/crypto"
)

func TestDualPasswordHasher_Argon2idRoundTrip(t *testing.T) {
	h := cryptoadapter.NewDualPasswordHasher()
	encoded, err := h.Hash("VedamLocal123!")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}

	ok, rehash, err := h.Verify("VedamLocal123!", encoded)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected password to match")
	}
	if rehash {
		t.Fatalf("argon2id should not request rehash")
	}

	bad, _, err := h.Verify("WrongPassword!", encoded)
	if err != nil || bad {
		t.Fatalf("expected wrong password rejection, got ok=%v err=%v", bad, err)
	}
}

func TestDualPasswordHasher_NodeScryptCompat(t *testing.T) {
	h := cryptoadapter.NewDualPasswordHasher()
	// Derived via Node crypto.scryptSync("VedamLocal123!", "0123456789abcdef0123456789abcdef", 64).toString("hex")
	nodeHash := "0123456789abcdef0123456789abcdef:1ca4040ad5bdacb8f930876c9550ed344057396a4c810de8ff853cc6b7e550a4bb9072d65846f9731afcf7ab137710a5303ee90a0eb750c3a35af6ad514a2bbb"
	ok, rehash, err := h.Verify("VedamLocal123!", nodeHash)
	if err != nil {
		t.Fatalf("scrypt verify error: %v", err)
	}
	if !ok {
		t.Fatalf("expected Node scrypt hash to verify")
	}
	if !rehash {
		t.Fatalf("expected legacy scrypt hash to flag needsRehash=true")
	}
}
