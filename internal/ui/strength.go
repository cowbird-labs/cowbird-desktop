package ui

import "math"

// passwordStrength returns a 0..1 score and a coarse label for pw, from a
// charset-entropy estimate with a repetition penalty. Advisory display only —
// it gates nothing; the unlock password's real protection is the Argon2id KDF.
func passwordStrength(pw string) (float64, string) {
	if pw == "" {
		return 0, ""
	}

	var lower, upper, digit, other bool
	unique := make(map[rune]struct{})
	n := 0
	for _, r := range pw {
		n++
		unique[r] = struct{}{}
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			other = true
		}
	}

	charset := 0
	if lower {
		charset += 26
	}
	if upper {
		charset += 26
	}
	if digit {
		charset += 10
	}
	if other {
		charset += 33
	}

	// Repeated characters carry less entropy than length suggests; average
	// the length with the unique-character count so "aaaaaaaa" scores low.
	effective := (float64(n) + float64(len(unique))) / 2
	bits := effective * math.Log2(float64(charset))

	return strengthFromBits(bits)
}

// strengthFromBits maps an entropy estimate (bits) to a 0..1 bar score and a
// coarse label, the shared scale used by both the typed-password heuristic and
// the generator's entropy-accurate readout.
func strengthFromBits(bits float64) (float64, string) {
	label := "Very weak"
	switch {
	case bits >= 80:
		label = "Strong"
	case bits >= 60:
		label = "Good"
	case bits >= 36:
		label = "Fair"
	case bits >= 28:
		label = "Weak"
	}
	return math.Min(bits/80, 1), label
}
