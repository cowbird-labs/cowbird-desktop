package crypto

import (
	"bytes"
	"testing"
)

func TestSealToSelfRoundTrip(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	plaintext := []byte(`{"labels":["work","email"],"favorite":true}`)

	sealed, err := SealToSelf(id, plaintext)
	if err != nil {
		t.Fatalf("SealToSelf: %v", err)
	}
	if bytes.Contains(sealed.Ciphertext, []byte("work")) {
		t.Fatal("ciphertext leaks plaintext")
	}

	got, err := OpenFromSelf(id, sealed)
	if err != nil {
		t.Fatalf("OpenFromSelf: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
	}
}

func TestSealToSelfEmpty(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	sealed, err := SealToSelf(id, nil)
	if err != nil {
		t.Fatalf("SealToSelf: %v", err)
	}
	got, err := OpenFromSelf(id, sealed)
	if err != nil {
		t.Fatalf("OpenFromSelf: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %q", got)
	}
}

func TestOpenFromSelfWrongIdentity(t *testing.T) {
	id, _ := NewIdentity()
	other, _ := NewIdentity()
	sealed, err := SealToSelf(id, []byte("secret"))
	if err != nil {
		t.Fatalf("SealToSelf: %v", err)
	}
	if _, err := OpenFromSelf(other, sealed); err == nil {
		t.Fatal("expected decryption failure with a different identity")
	}
}

func TestOpenFromSelfTampered(t *testing.T) {
	id, _ := NewIdentity()
	sealed, err := SealToSelf(id, []byte("secret"))
	if err != nil {
		t.Fatalf("SealToSelf: %v", err)
	}
	sealed.Ciphertext[0] ^= 0xff
	if _, err := OpenFromSelf(id, sealed); err == nil {
		t.Fatal("expected decryption failure on tampered ciphertext")
	}
}
