package crypto

import (
	"bytes"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key, err := NewItemKey()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("hello, cowbird")

	nonce, ciphertext, err := Seal(key, plaintext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nonce) == 0 || len(ciphertext) == 0 {
		t.Fatal("expected non-empty nonce and ciphertext")
	}

	got, err := Open(key, nonce, ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypted %q, want %q", got, plaintext)
	}
}

func TestSealProducesDistinctCiphertexts(t *testing.T) {
	key, _ := NewItemKey()
	plaintext := []byte("same plaintext")

	_, c1, _ := Seal(key, plaintext, nil)
	_, c2, _ := Seal(key, plaintext, nil)
	if bytes.Equal(c1, c2) {
		t.Fatal("two Seal calls must produce distinct ciphertexts (random nonce)")
	}
}

func TestOpenWrongKey(t *testing.T) {
	key, _ := NewItemKey()
	nonce, ciphertext, _ := Seal(key, []byte("secret"), nil)

	wrongKey, _ := NewItemKey()
	_, err := Open(wrongKey, nonce, ciphertext, nil)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestSealOpenWithAAD(t *testing.T) {
	key, _ := NewItemKey()
	plaintext := []byte("bound content")
	aad := []byte("owner-id\x00login")

	nonce, ciphertext, err := Seal(key, plaintext, aad)
	if err != nil {
		t.Fatal(err)
	}

	// Correct aad opens.
	got, err := Open(key, nonce, ciphertext, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypted %q, want %q", got, plaintext)
	}

	// Tampered aad (e.g. an operator changing the bound owner/type) fails.
	if _, err := Open(key, nonce, ciphertext, []byte("owner-id\x00card")); err == nil {
		t.Fatal("expected error opening with altered aad")
	}
	// Dropping the aad entirely also fails — the binding is mandatory.
	if _, err := Open(key, nonce, ciphertext, nil); err == nil {
		t.Fatal("expected error opening without the aad")
	}
}

func TestOpenTamperedCiphertext(t *testing.T) {
	key, _ := NewItemKey()
	nonce, ciphertext, _ := Seal(key, []byte("secret"), nil)

	ciphertext[0] ^= 0xff
	_, err := Open(key, nonce, ciphertext, nil)
	if err == nil {
		t.Fatal("expected error decrypting tampered ciphertext")
	}
}