// Package generate produces cryptographically strong random passwords and
// word-based passphrases. It is independent of the UI so the GUI and a future
// CLI can share it. All randomness comes from crypto/rand via unbiased
// rejection sampling (see rand.go); math/rand is never used.
package generate

import (
	"fmt"
	"math"
	"strings"
)

// Character classes for password generation.
const (
	lowerChars  = "abcdefghijklmnopqrstuvwxyz"
	upperChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitChars  = "0123456789"
	symbolChars = "!@#$%^&*()-_=+[]{}<>?/"

	// ambiguousChars are visually confusable characters dropped when
	// PasswordOpts.ExcludeAmbiguous is set.
	ambiguousChars = "il1IoO0|"
)

// PasswordOpts configures random-character password generation.
type PasswordOpts struct {
	Length           int
	Lower            bool
	Upper            bool
	Digits           bool
	Symbols          bool
	ExcludeAmbiguous bool
}

// PassphraseOpts configures word-based passphrase generation.
type PassphraseOpts struct {
	Words         int
	Separator     string
	Capitalize    bool
	IncludeNumber bool
}

// classes returns the enabled character classes, each already stripped of
// ambiguous characters when requested. Empty classes (none enabled) yield nil.
func (o PasswordOpts) classes() []string {
	var cs []string
	for _, c := range []struct {
		on    bool
		chars string
	}{
		{o.Lower, lowerChars},
		{o.Upper, upperChars},
		{o.Digits, digitChars},
		{o.Symbols, symbolChars},
	} {
		if !c.on {
			continue
		}
		s := c.chars
		if o.ExcludeAmbiguous {
			s = stripChars(s, ambiguousChars)
		}
		if s != "" {
			cs = append(cs, s)
		}
	}
	return cs
}

// Password returns a random password honouring opts. It guarantees at least one
// character from each enabled class, then fills the remaining length from the
// combined pool and shuffles so the guaranteed characters are not positionally
// fixed. It errors if no class is enabled or the length cannot accommodate one
// character per enabled class.
func Password(opts PasswordOpts) (string, error) {
	classes := opts.classes()
	if len(classes) == 0 {
		return "", fmt.Errorf("password: at least one character class must be enabled")
	}
	if opts.Length < len(classes) {
		return "", fmt.Errorf("password: length %d is too short for %d character classes", opts.Length, len(classes))
	}

	pool := strings.Join(classes, "")
	out := make([]byte, 0, opts.Length)

	// One guaranteed character from each enabled class.
	for _, class := range classes {
		c, err := randPick([]byte(class))
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}
	// Remainder from the combined pool.
	poolBytes := []byte(pool)
	for len(out) < opts.Length {
		c, err := randPick(poolBytes)
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}

	if err := shuffle(out); err != nil {
		return "", err
	}
	return string(out), nil
}

// Passphrase returns a separator-joined sequence of words drawn from the EFF
// long wordlist. With Capitalize each word is title-cased; with IncludeNumber a
// single random digit is appended to one randomly chosen word. It errors if
// Words is not positive.
func Passphrase(opts PassphraseOpts) (string, error) {
	if opts.Words <= 0 {
		return "", fmt.Errorf("passphrase: word count must be positive, got %d", opts.Words)
	}

	chosen := make([]string, opts.Words)
	for i := range chosen {
		w, err := randPick(words)
		if err != nil {
			return "", err
		}
		if opts.Capitalize {
			w = capitalize(w)
		}
		chosen[i] = w
	}

	if opts.IncludeNumber {
		d, err := randIndex(10)
		if err != nil {
			return "", err
		}
		idx, err := randIndex(len(chosen))
		if err != nil {
			return "", err
		}
		chosen[idx] += fmt.Sprintf("%d", d)
	}

	return strings.Join(chosen, opts.Separator), nil
}

// Entropy reports the password's entropy in bits, length × log2(poolSize) over
// the combined enabled-class pool. Returns 0 when no class is enabled.
func (o PasswordOpts) Entropy() float64 {
	pool := 0
	for _, c := range o.classes() {
		pool += len(c)
	}
	if pool == 0 || o.Length == 0 {
		return 0
	}
	return float64(o.Length) * math.Log2(float64(pool))
}

// Entropy reports the passphrase's entropy in bits from the word choices alone
// (Words × log2(WordlistSize)). Capitalization and the optional digit add a
// little more but are not counted, so this is a conservative figure.
func (o PassphraseOpts) Entropy() float64 {
	if o.Words <= 0 {
		return 0
	}
	return float64(o.Words) * math.Log2(float64(WordlistSize))
}

// stripChars returns s with every character that appears in remove deleted.
func stripChars(s, remove string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(remove, r) {
			return -1
		}
		return r
	}, s)
}

// capitalize upper-cases the first letter of w (ASCII; the wordlist is ASCII).
func capitalize(w string) string {
	if w == "" {
		return w
	}
	return strings.ToUpper(w[:1]) + w[1:]
}
