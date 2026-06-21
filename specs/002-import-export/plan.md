# Implementation Plan: Item Import and Export

**Branch**: `002-import-export` | **Spec**: [spec.md](./spec.md)

## Approach

Three layers, matching the existing UI/core decoupling:

1. **Format** (`internal/items/transfer.go`): the on-disk export document and its
   encode/decode. Lives in `items` because it is purely about item contents and
   reuses the existing `Encode`/`Decode` `{type, data}` envelope per entry. No
   crypto, no Vault, no Service dependency — trivially unit-testable.

2. **Orchestration** (`internal/core/transfer.go`): `App.ExportItems` and
   `App.ImportItems`. Export lists owned envelopes, decrypts each with
   `Service.OpenOwnItem`, skips undecryptable ones, and hands the contents to the
   format encoder. Import decodes the file and calls `Service.CreateItem` per
   entry, tallying successes and skips. These take a `context.Context` and return
   bytes / a result struct, so a future CLI can call them directly.

3. **UI** (`internal/ui/transfer.go`): two new hamburger-menu actions wired in
   `showMainMenu` (`internal/ui/password.go`):
   - **Export Items…** → unencrypted-file warning confirm → save dialog →
     `App.ExportItems` on a worker goroutine → write bytes → success/error via
     `fyne.Do`.
   - **Import Items…** → open dialog → read bytes → `App.ImportItems` → report
     `N imported, M skipped` → refresh the list.

## Data shapes

```go
// internal/items/transfer.go
const ExportFormat = "cowbird-export"
const ExportVersion = 1

type ExportFile struct {
    Format     string          `json:"format"`      // ExportFormat
    Version    int             `json:"version"`     // ExportVersion
    ExportedAt time.Time       `json:"exported_at"`
    Items      []json.RawMessage `json:"items"`     // each is an Encode() envelope
}

func EncodeExport(contents []Content) ([]byte, error)
func DecodeExport(b []byte) ([]Content, error)   // validates Format + Version
```

```go
// internal/core/transfer.go
type ImportResult struct {
    Imported int
    Skipped  int
}

func (a *App) ExportItems(ctx context.Context) ([]byte, error)
func (a *App) ImportItems(ctx context.Context, data []byte) (ImportResult, error)
```

## Decisions

- **Plaintext JSON, indented.** Human-inspectable and diff-friendly; the UI warns
  it is unprotected. Passphrase encryption is deferred (would reuse
  `crypto.ExportKey`'s Argon2id + XChaCha20 path).
- **Skip-don't-abort** on both sides: an undecryptable owned item (export) or an
  unrecognised entry (import) is counted and skipped, never fatal — mirrors
  `loadRows` turning decrypt failures into "unreadable" rows rather than errors.
- **Whole-file validation before any write** on import: `DecodeExport` rejects a
  bad format tag / version up front, so a malformed file writes nothing.

## Tests

- `items/transfer_test.go`: round-trip every item type incl. custom fields;
  reject wrong format tag, wrong version, malformed JSON; tolerate an entry with
  an unknown item type (skipped).
- `core/transfer_test.go`: export→import round-trip against the in-memory Store
  used by `sharing` tests; verify owned-only export and import success/skip
  counts.

## Out of Scope (this plan)

Foreign formats, CSV, shared-in items, encrypted export files, CLI command,
import de-duplication. See spec Out of Scope.
