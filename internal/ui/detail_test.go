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

	// Full otpauth:// URIs (as imported from other managers) are parsed for the
	// secret and parameters rather than treated as a bare secret. Uses the public
	// RFC 6238 test-vector secret, not a real account secret.
	uri := "otpauth://totp/Example:alice%40example.com?issuer=Example&secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&algorithm=SHA1&digits=6&period=30"
	code, remaining, err = totpNow(uri)
	if err != nil {
		t.Fatalf("otpauth URI: unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("otpauth URI code = %q, want 6 digits", code)
	}
	if remaining < 1 || remaining > 30 {
		t.Errorf("otpauth URI remaining = %d, want 1..30", remaining)
	}

	// A URI with non-default digits/period is honored.
	uri8 := "otpauth://totp/x?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&digits=8&period=60"
	code, remaining, err = totpNow(uri8)
	if err != nil {
		t.Fatalf("8-digit URI: unexpected error: %v", err)
	}
	if len(code) != 8 {
		t.Errorf("8-digit URI code = %q, want 8 digits", code)
	}
	if remaining < 1 || remaining > 60 {
		t.Errorf("60s-period URI remaining = %d, want 1..60", remaining)
	}
}
