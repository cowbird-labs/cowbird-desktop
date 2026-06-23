package vault

import "testing"

func TestValidateAddress(t *testing.T) {
	ok := []string{
		"https://vault.avitac.co:8200",
		"https://localhost:8200",
		"http://localhost:8200", // loopback dev exception
		"http://127.0.0.1:8200", // loopback dev exception
		"http://[::1]:8200",     // IPv6 loopback dev exception
	}
	for _, a := range ok {
		if err := validateAddress(a); err != nil {
			t.Errorf("validateAddress(%q) = %v, want nil", a, err)
		}
	}

	bad := []string{
		"http://vault.avitac.co:8200", // cleartext to a remote host
		"http://10.0.0.5:8200",        // private but not loopback
		"ftp://vault.avitac.co",       // wrong scheme
		"vault.avitac.co:8200",        // no scheme
	}
	for _, a := range bad {
		if err := validateAddress(a); err == nil {
			t.Errorf("validateAddress(%q) = nil, want error", a)
		}
	}
}
