package generate

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// randIndex returns a uniformly distributed integer in [0, n) drawn from
// crypto/rand. It uses rejection sampling against the largest multiple of n
// that fits in a uint64, so the result carries no modulo bias. n must be > 0.
func randIndex(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("randIndex: n must be positive, got %d", n)
	}
	un := uint64(n)
	// limit is the largest multiple of un not exceeding 2^64-1. Values at or
	// above it (the top 2^64 mod un values) would skew the distribution, so
	// they are rejected and re-drawn.
	maxUint := ^uint64(0)
	limit := maxUint - (maxUint % un)
	var buf [8]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, fmt.Errorf("reading random bytes: %w", err)
		}
		v := binary.BigEndian.Uint64(buf[:])
		if v < limit {
			return int(v % un), nil
		}
	}
}

// randPick returns a uniformly random element of s. s must be non-empty.
func randPick[T any](s []T) (T, error) {
	var zero T
	i, err := randIndex(len(s))
	if err != nil {
		return zero, err
	}
	return s[i], nil
}

// shuffle performs an in-place Fisher–Yates shuffle of s using crypto/rand, so
// the positions of guaranteed characters in a generated password are not fixed.
func shuffle[T any](s []T) error {
	for i := len(s) - 1; i > 0; i-- {
		j, err := randIndex(i + 1)
		if err != nil {
			return err
		}
		s[i], s[j] = s[j], s[i]
	}
	return nil
}
