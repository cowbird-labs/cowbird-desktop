package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

// SaltLen is the length of the random salt fed to the KDF.
const SaltLen = 32

// kdfParams are the Argon2id work factors for one KDF version.
type kdfParams struct {
	time    uint32
	memory  uint32 // KiB
	threads uint8
	keyLen  uint32
}

// KDF versions. The set is append-only: every locked record stores the version
// it was derived under (see LockedIdentity.Version / ExportedKey.KDFVersion), so
// raising the defaults never invalidates existing records. Version 0 (absent)
// denotes a record written before versioning existed and is treated as kdfV1.
//
// Parameters are kept mobile-feasible: memory stays at 64 MiB (a 256 MiB+ profile
// risks OOM on phones), and strength is bought with iterations instead. kdfV2
// raises time 3 -> 25 (~0.4 s/unlock on desktop, ~1-2 s on mobile).
const (
	kdfV1 = 1
	kdfV2 = 2

	// currentKDFVersion is the version new records are locked under.
	currentKDFVersion = kdfV2
)

var kdfVersions = map[int]kdfParams{
	kdfV1: {time: 3, memory: 64 * 1024, threads: 4, keyLen: 32},
	kdfV2: {time: 25, memory: 64 * 1024, threads: 4, keyLen: 32},
}

// kdfParamsForVersion returns the parameters for a stored KDF version, mapping
// the absent version 0 to kdfV1.
func kdfParamsForVersion(v int) (kdfParams, error) {
	if v == 0 {
		v = kdfV1
	}
	p, ok := kdfVersions[v]
	if !ok {
		return kdfParams{}, fmt.Errorf("unknown KDF version %d", v)
	}
	return p, nil
}

// GenerateSalt returns a 32-byte cryptographically random salt.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// DeriveUnlockKey derives a 32-byte key from the unlock password and salt using
// the CURRENT KDF version's parameters. Lock paths use this; unlock re-derives
// with the parameters the record was written under (see UnlockIdentity).
func DeriveUnlockKey(password, salt []byte) []byte {
	p, err := kdfParamsForVersion(currentKDFVersion)
	if err != nil {
		panic(err) // currentKDFVersion is always present in the table
	}
	return deriveUnlockKey(password, salt, p)
}

// deriveUnlockKey runs Argon2id with the given parameters, then HKDF for domain
// separation (info "cowbird-unlock-v1"), yielding the 32-byte unlock key.
func deriveUnlockKey(password, salt []byte, p kdfParams) []byte {
	master := argon2.IDKey(password, salt, p.time, p.memory, p.threads, p.keyLen)
	r := hkdf.New(sha256.New, master, salt, []byte("cowbird-unlock-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		panic(err) // unreachable: HKDF does not fail with valid inputs
	}
	return key
}