// Package transfer provides bidirectional adapters ("codecs") between cowbird's
// item model and the import/export file formats of other password managers. It
// depends only on internal/items, so both the GUI and a future CLI can share it;
// crypto and Vault orchestration stay in internal/core.
package transfer

import "cowbird/internal/items"

// Codec is a named, bidirectional adapter for one file format. Marshal turns
// cowbird items into the format's bytes; Unmarshal parses bytes back into
// cowbird items, returning a count of entries that were skipped because they
// could not be mapped (a malformed or unrepresentable entry is skipped, not
// fatal — consistent with the native import).
type Codec interface {
	ID() string        // stable identifier, e.g. "bitwarden"
	Name() string      // human label for the UI, e.g. "Bitwarden (JSON)"
	Extension() string // default file extension including the dot, e.g. ".json"
	Marshal(contents []items.Content) ([]byte, error)
	Unmarshal(data []byte) (contents []items.Content, skipped int, err error)
}

// codecs is the ordered registry. Cowbird-native is first (the default); the
// vendor formats follow.
var codecs = []Codec{
	cowbirdCodec{},
	bitwardenCodec{},
	onePasswordCodec{},
	protonCodec{},
	lastPassCodec{},
}

// All returns the available codecs in display order.
func All() []Codec { return codecs }

// Get returns the codec with the given ID.
func Get(id string) (Codec, bool) {
	for _, c := range codecs {
		if c.ID() == id {
			return c, true
		}
	}
	return nil, false
}

// Default returns the codec used when none is chosen (cowbird-native JSON).
func Default() Codec { return codecs[0] }
