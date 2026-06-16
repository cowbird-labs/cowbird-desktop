package ui

import "testing"

func TestGroupDigits(t *testing.T) {
	cases := []struct {
		in   string
		size int
		want string
	}{
		{"4242424242424242", 4, "4242 4242 4242 4242"}, // 16-digit card
		{"378282246310005", 4, "3782 8224 6310 005"},   // 15-digit Amex, short final group
		{"123456", 3, "123 456"},                       // TOTP code
		{"12", 4, "12"},                                // shorter than a group
		{"", 4, ""},                                    // empty
		{"1234", 0, "1234"},                            // non-positive size is a no-op
	}
	for _, c := range cases {
		if got := groupDigits(c.in, c.size); got != c.want {
			t.Errorf("groupDigits(%q, %d) = %q, want %q", c.in, c.size, got, c.want)
		}
	}
}

func TestTotpNow(t *testing.T) {
	// RFC 6238 test vector secret, base32 of "12345678901234567890".
	code, remaining, err := totpNow("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("code = %q, want 6 digits", code)
	}
	if remaining < 1 || remaining > 30 {
		t.Errorf("remaining = %d, want 1..30", remaining)
	}

	// Internal spaces are tolerated (grouped secrets).
	if _, _, err := totpNow("GEZD GNBV GY3T QOJQ GEZD GNBV GY3T QOJQ"); err != nil {
		t.Errorf("spaced secret: unexpected error: %v", err)
	}

	// Empty and malformed secrets error rather than panicking.
	if _, _, err := totpNow("   "); err == nil {
		t.Error("empty secret: expected error, got nil")
	}
	if _, _, err := totpNow("not-base32!!"); err == nil {
		t.Error("malformed secret: expected error, got nil")
	}
}
