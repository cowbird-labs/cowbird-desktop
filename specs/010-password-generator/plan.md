# Implementation Plan: Password & Passphrase Generator

**Branch**: `010-password-generator` | **Spec**: [spec.md](./spec.md)
**Status**: Draft

Same stack as the rest of the project (Go 1.26, Fyne, Viper TOML config). The
generator core is a new UI-independent package; the UI wires it into the editor
and the main menu.

## Design

### New package: `internal/generate`

UI-independent, like `crypto` and `items`, so the planned CLI can reuse it.

```go
type PasswordOpts struct {
	Length          int
	Lower, Upper    bool
	Digits, Symbols bool
	ExcludeAmbiguous bool
}

type PassphraseOpts struct {
	Words         int
	Separator     string
	Capitalize    bool
	IncludeNumber bool
}

func Password(opts PasswordOpts) (string, error)
func Passphrase(opts PassphraseOpts) (string, error)

// Entropy reports the bits of entropy for a given option set, for the UI
// readout (independent of any one generated sample).
func (o PasswordOpts) Entropy() float64    // length * log2(poolSize)
func (o PassphraseOpts) Entropy() float64  // words * log2(WordlistSize)
```

- **Randomness**: a small `randInt(n)` helper over `crypto/rand` using rejection
  sampling (read a uniform value in `[0, n)` discarding biased high values), and
  `randShuffle` for the class-coverage step. No `math/rand`. Selection is
  index-into-pool, never modulo.
- **Password algorithm**: build the character pool from the enabled classes
  (minus the ambiguous set `O0oIl1|` etc. when `ExcludeAmbiguous`). Place one
  guaranteed char from each enabled class first, fill the remainder from the
  full pool, then shuffle so the guaranteed chars aren't positionally fixed.
  Validate: at least one class enabled; `Length >= number of enabled classes`
  (otherwise the guarantee can't hold — return an error the UI surfaces / clamps).
- **Passphrase algorithm**: pick `Words` indices into the wordlist, optionally
  title-case each, join with `Separator`; if `IncludeNumber`, append/insert a
  single random digit at a random word boundary. Entropy counts only the word
  choices (the digit/caps are minor, not advertised as the headline number).
- **Wordlist**: `eff_large_wordlist.txt` embedded via `go:embed`; parsed once
  into `[]string` (the EFF file is `<dice>\t<word>` lines — keep only the word).
  `WordlistSize` constant = 7776 (`log2 ≈ 12.925` bits/word).

### Config (`internal/config`)

A new sub-config with `creasty/defaults` tags (defaults defined once, never in
`viper.SetDefault`), following the existing sub-config convention:

```go
type Generator struct {
	Mode       string `mapstructure:"mode" default:"password"`
	Length     int    `mapstructure:"length" default:"20"`
	Lower      bool   `mapstructure:"lower" default:"true"`
	Upper      bool   `mapstructure:"upper" default:"true"`
	Digits     bool   `mapstructure:"digits" default:"true"`
	Symbols    bool   `mapstructure:"symbols" default:"true"`
	ExcludeAmbiguous bool `mapstructure:"exclude_ambiguous" default:"false"`
	Words      int    `mapstructure:"words" default:"5"`
	Separator  string `mapstructure:"separator" default:"-"`
	Capitalize bool   `mapstructure:"capitalize" default:"true"`
	IncludeNumber bool `mapstructure:"include_number" default:"true"`
}
```

Added as `Config.Generator`. Persisted through the existing
`config.Save(cfg)` (Viper + `ConfigFileUsed()`). The UI maps between these flat
config fields and the `generate.*Opts` structs.

### UI (`internal/ui`)

- **`ui/generator.go` (new)**: one reusable generator widget/dialog. Renders a
  mode toggle (Password / Passphrase), the relevant option controls, the
  generated value (in a copyable, reveal-capable field), a strength/entropy
  readout, and Regenerate. Takes an optional `onUse func(string)` callback:
  - inline (from the editor): `onUse` sets the target entry's text, then closes.
  - standalone (from the menu): no `onUse`; shows a Copy button instead.
  Reads initial options from `config.Generator`; on generate/close, writes the
  current options back and calls `config.Save` (best-effort; persistence failure
  is non-fatal and logged, not blocking).
- **Inline hook (`ui/fields.go` + `ui/editor.go`)**: add a `generatable bool`
  flag to `fieldSpec` (builder method `.gen()`), set on `Login.Password` and
  `Password.Password`. In `showEditor`, sensitive entries whose spec is
  `generatable` get a small generate button (dice/refresh icon) wired to open
  the generator dialog with `onUse` filling that entry. Hidden custom-field rows
  (`FieldHidden`) get the same button in their value cell.
- **Standalone hook (`ui/app.go` `showMainMenu`)**: a "Generate password…" menu
  item alongside the existing change-password / export / rotate entries, opening
  the generator dialog in copy mode.
- **Strength readout**: reuse `passwordStrength` (`ui/strength.go`) for password
  mode; for passphrase mode show `PassphraseOpts.Entropy()` bits with a coarse
  label so the meter isn't fooled by the separator-laden string (FR-005).

## Build Order (risk-first)

1. `internal/generate`: opts structs, `crypto/rand` helpers, `Password`,
   `Passphrase`, `Entropy`, embedded wordlist — with tests. No UI.
2. `internal/config`: `Generator` sub-config + defaults; round-trip through Save.
3. `ui/generator.go` reusable dialog; wire standalone menu entry first (simplest
   path), then the inline editor/custom-field buttons.

## Tests (`internal/generate`)

- Password respects length and enabled classes; output contains ≥1 char from
  each enabled class; never contains ambiguous chars when excluded; errors when
  `Length < enabledClasses` or zero classes.
- Passphrase has the right word count, all words are from the list, separator
  and capitalization applied, a digit present when requested.
- Bias smoke test: large sample of `randInt(n)` covers the full `[0,n)` range
  roughly uniformly (sanity, not a statistical proof).
- `Entropy()` matches the closed-form for known option sets.
- UI/config layers untested per project precedent (config Save needs a file;
  UI needs Fyne) — covered indirectly.

## Notes / follow-ups

- EFF wordlist license/attribution: add a short `NOTICE`/comment crediting EFF
  (CC-BY) next to the embedded file.
- A future CLI `generate` subcommand can call `internal/generate` directly.
- Breach-check and password history are out of scope (see spec).
