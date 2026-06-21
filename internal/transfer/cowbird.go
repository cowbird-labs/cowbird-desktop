package transfer

import "cowbird/internal/items"

// cowbirdCodec is the native, full-fidelity format. It delegates to the export
// codec in internal/items, which owns the on-disk schema.
type cowbirdCodec struct{}

func (cowbirdCodec) ID() string        { return "cowbird" }
func (cowbirdCodec) Name() string      { return "Cowbird (JSON)" }
func (cowbirdCodec) Extension() string { return ".json" }

func (cowbirdCodec) Marshal(contents []items.Content) ([]byte, error) {
	return items.EncodeExport(contents)
}

func (cowbirdCodec) Unmarshal(data []byte) ([]items.Content, int, error) {
	return items.DecodeExport(data)
}
