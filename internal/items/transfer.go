package items

import (
	"encoding/json"
	"fmt"
	"time"
)

// ExportFormat tags a cowbird export document so import can reject foreign or
// unrelated JSON before attempting to decode any items.
const ExportFormat = "cowbird-export"

// ExportVersion is the current export schema version. Bump it on any
// incompatible change to ExportFile; DecodeExport rejects versions it does not
// understand.
const ExportVersion = 1

// ExportFile is the on-disk representation of a bulk item export. Each entry in
// Items is a self-describing {type, data} envelope produced by Encode, so the
// document round-trips losslessly through DecodeExport.
type ExportFile struct {
	Format     string            `json:"format"`
	Version    int               `json:"version"`
	ExportedAt time.Time         `json:"exported_at"`
	Items      []json.RawMessage `json:"items"`
}

// EncodeExport serializes contents into an indented cowbird export document.
// Each Content is encoded with the standard item codec, so the result decodes
// back to the same concrete types via DecodeExport.
func EncodeExport(contents []Content) ([]byte, error) {
	entries := make([]json.RawMessage, 0, len(contents))
	for _, c := range contents {
		b, err := Encode(c)
		if err != nil {
			return nil, fmt.Errorf("encoding export entry: %w", err)
		}
		entries = append(entries, b)
	}
	file := ExportFile{
		Format:     ExportFormat,
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Items:      entries,
	}
	return json.MarshalIndent(file, "", "  ")
}

// DecodeExport parses a cowbird export document and returns its items. The
// whole document is validated (format tag and version) before any entry is
// decoded, so a file cowbird does not recognise is rejected without partial
// results. Individual entries that fail to decode are skipped and reported via
// the returned skipped count rather than aborting the import.
func DecodeExport(b []byte) (contents []Content, skipped int, err error) {
	var file ExportFile
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, 0, fmt.Errorf("parsing export file: %w", err)
	}
	if file.Format != ExportFormat {
		return nil, 0, fmt.Errorf("not a cowbird export file (format %q)", file.Format)
	}
	if file.Version != ExportVersion {
		return nil, 0, fmt.Errorf("unsupported export version %d (expected %d)", file.Version, ExportVersion)
	}

	contents = make([]Content, 0, len(file.Items))
	for _, raw := range file.Items {
		c, err := Decode(raw)
		if err != nil {
			skipped++
			continue
		}
		contents = append(contents, c)
	}
	return contents, skipped, nil
}
