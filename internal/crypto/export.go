package crypto

import (
	"encoding/json"
	"fmt"
)

const exportVersion = 1

// ExportedKey is the passphrase-protected recovery file format.
// Users should store this in a safe location outside of the device.
// Version is the export-file format version; KDFVersion is the Argon2id
// parameter-set version used to derive the encryption key (0/absent = kdfV1, for
// recovery files written before KDF versioning).
type ExportedKey struct {
	Version    int    `json:"version"`
	KDFVersion int    `json:"kdf_version,omitempty"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// ExportKey serializes and encrypts the identity's private keys under passphrase.
// The returned bytes should be written to a file and kept in a safe location.
// This is the only recovery mechanism if the device is lost.
func ExportKey(id *Identity, passphrase []byte) ([]byte, error) {
	locked, err := LockIdentity(id, passphrase)
	if err != nil {
		return nil, fmt.Errorf("locking identity: %w", err)
	}
	exported := ExportedKey{
		Version:    exportVersion,
		KDFVersion: locked.Version,
		Salt:       locked.Salt,
		Nonce:      locked.Nonce,
		Ciphertext: locked.Ciphertext,
	}
	data, err := json.Marshal(exported)
	if err != nil {
		return nil, fmt.Errorf("encoding export: %w", err)
	}
	return data, nil
}

// ImportKey parses and decrypts an exported key file, restoring the Identity.
func ImportKey(data, passphrase []byte) (*Identity, error) {
	var exported ExportedKey
	if err := json.Unmarshal(data, &exported); err != nil {
		return nil, fmt.Errorf("parsing export file: %w", err)
	}
	if exported.Version != exportVersion {
		return nil, fmt.Errorf("unsupported export version %d", exported.Version)
	}
	locked := &LockedIdentity{
		Version:    exported.KDFVersion,
		Salt:       exported.Salt,
		Nonce:      exported.Nonce,
		Ciphertext: exported.Ciphertext,
	}
	id, err := UnlockIdentity(locked, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting export: %w", err)
	}
	return id, nil
}