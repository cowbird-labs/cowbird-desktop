package generate

import (
	"math"
	"strings"
	"testing"
)

func TestPasswordLengthAndClasses(t *testing.T) {
	opts := PasswordOpts{Length: 24, Lower: true, Upper: true, Digits: true, Symbols: true}
	pw, err := Password(opts)
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	if len([]byte(pw)) != 24 {
		t.Fatalf("length = %d, want 24", len(pw))
	}
	if !strings.ContainsAny(pw, lowerChars) {
		t.Errorf("missing lowercase in %q", pw)
	}
	if !strings.ContainsAny(pw, upperChars) {
		t.Errorf("missing uppercase in %q", pw)
	}
	if !strings.ContainsAny(pw, digitChars) {
		t.Errorf("missing digit in %q", pw)
	}
	if !strings.ContainsAny(pw, symbolChars) {
		t.Errorf("missing symbol in %q", pw)
	}
}

func TestPasswordOnlyEnabledClasses(t *testing.T) {
	opts := PasswordOpts{Length: 16, Digits: true}
	pw, err := Password(opts)
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	for _, r := range pw {
		if !strings.ContainsRune(digitChars, r) {
			t.Fatalf("non-digit %q in digits-only password %q", r, pw)
		}
	}
}

func TestPasswordExcludeAmbiguous(t *testing.T) {
	opts := PasswordOpts{Length: 200, Lower: true, Upper: true, Digits: true, ExcludeAmbiguous: true}
	pw, err := Password(opts)
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	if strings.ContainsAny(pw, ambiguousChars) {
		t.Fatalf("ambiguous character present in %q", pw)
	}
}

func TestPasswordErrors(t *testing.T) {
	if _, err := Password(PasswordOpts{Length: 10}); err == nil {
		t.Error("expected error with no classes enabled")
	}
	// Length too short for the number of enabled classes (4 classes, length 3).
	if _, err := Password(PasswordOpts{Length: 3, Lower: true, Upper: true, Digits: true, Symbols: true}); err == nil {
		t.Error("expected error when length < enabled classes")
	}
}

func TestPassphraseBasics(t *testing.T) {
	opts := PassphraseOpts{Words: 5, Separator: "-", Capitalize: false}
	p, err := Passphrase(opts)
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	parts := strings.Split(p, "-")
	if len(parts) != 5 {
		t.Fatalf("got %d words, want 5: %q", len(parts), p)
	}
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		set[w] = struct{}{}
	}
	for part := range strings.SplitSeq(p, "-") {
		if _, ok := set[part]; !ok {
			t.Errorf("word %q not in wordlist", part)
		}
	}
}

func TestPassphraseCapitalizeAndNumber(t *testing.T) {
	opts := PassphraseOpts{Words: 6, Separator: ".", Capitalize: true, IncludeNumber: true}
	p, err := Passphrase(opts)
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	if !strings.ContainsAny(p, digitChars) {
		t.Errorf("expected a digit in %q", p)
	}
	for part := range strings.SplitSeq(p, ".") {
		// Strip a trailing digit the include-number step may have appended.
		trimmed := strings.TrimRight(part, digitChars)
		if trimmed == "" {
			continue
		}
		if c := trimmed[0]; c < 'A' || c > 'Z' {
			t.Errorf("word %q not capitalized in %q", part, p)
		}
	}
}

func TestPassphraseErrors(t *testing.T) {
	if _, err := Passphrase(PassphraseOpts{Words: 0, Separator: "-"}); err == nil {
		t.Error("expected error for zero words")
	}
}

func TestWordlistSize(t *testing.T) {
	if len(words) != WordlistSize {
		t.Fatalf("wordlist has %d words, want %d", len(words), WordlistSize)
	}
}

func TestEntropy(t *testing.T) {
	pw := PasswordOpts{Length: 20, Lower: true, Upper: true, Digits: true, Symbols: true}
	want := 20 * math.Log2(float64(26+26+10+len(symbolChars)))
	if got := pw.Entropy(); math.Abs(got-want) > 1e-9 {
		t.Errorf("password entropy = %v, want %v", got, want)
	}

	pp := PassphraseOpts{Words: 6}
	wantPP := 6 * math.Log2(float64(WordlistSize))
	if got := pp.Entropy(); math.Abs(got-wantPP) > 1e-9 {
		t.Errorf("passphrase entropy = %v, want %v", got, wantPP)
	}
}

func TestRandIndexDistribution(t *testing.T) {
	const n = 7
	const draws = 7000
	counts := make([]int, n)
	for range draws {
		idx, err := randIndex(n)
		if err != nil {
			t.Fatalf("randIndex: %v", err)
		}
		if idx < 0 || idx >= n {
			t.Fatalf("randIndex out of range: %d", idx)
		}
		counts[idx]++
	}
	// Each bucket should land near draws/n; allow a generous ±40% band — this is
	// a smoke test for gross bias, not a statistical proof.
	expected := draws / n
	for i, c := range counts {
		if c < expected*6/10 || c > expected*14/10 {
			t.Errorf("bucket %d count %d far from expected %d", i, c, expected)
		}
	}
}
