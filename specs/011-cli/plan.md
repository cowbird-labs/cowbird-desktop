# Implementation Plan: CLI Command Scaffold

**Branch**: `011-cli` | **Spec**: [spec.md](./spec.md)
**Status**: Draft

Same stack as the rest of the project (Go 1.26, single binary). Adds
`github.com/spf13/cobra` and restructures the program entry point so a Cobra
command tree dispatches subcommands, with the no-argument path still launching
the existing Fyne GUI. The only leaf command implemented is `version`.

## Design

### New package: `internal/cli`

UI-independent, like `core`/`crypto`/`generate`. Holds the command tree so
`main.go` stays thin and so future data subcommands can import `core`/`vault`
without the GUI being forced to depend on command-layer code.

```go
// version is the build-time version, overridden via:
//   go build -ldflags "-X cowbird/internal/cli.version=0.7.0"
// Defaults to "dev" for a plain `go build`.
var version = "dev"

// Execute builds the root command and runs it. runGUI is the existing GUI
// bootstrap, invoked when no subcommand is given. Returns an error on an
// unknown command or subcommand failure (main maps that to a non-zero exit).
func Execute(runGUI func()) error
```

- **`cli.go`** — `Execute(runGUI)` constructs the root `cobra.Command`:
  ```go
  root := &cobra.Command{
      Use:   "cowbird",
      Short: "Cowbird password manager",
      // No subcommand → existing GUI flow.
      Run:           func(cmd *cobra.Command, args []string) { runGUI() },
      SilenceUsage:  true, // unknown-command error is enough; don't dump usage
  }
  root.AddCommand(newVersionCmd())
  return root.Execute()
  ```
  Cobra dispatches: bare `cowbird` runs `root.Run` (GUI); `cowbird version` runs
  the leaf; `cowbird frobnicate` returns an "unknown command" error (FR-006);
  `help`/`--help`/`completion` come for free (FR-007).
- **`version.go`** — `newVersionCmd()` returns a `cobra.Command{Use: "version"}`
  whose `Run` prints the version line(s) to `cmd.OutOrStdout()` (writing to the
  command's writer, not bare `fmt.Println`, keeps it testable). It reads no
  config/keyring/Vault (FR-004). Build metadata comes from
  `runtime/debug.ReadBuildInfo()`:
  - `version` var (linker-injected) is the headline.
  - From `bi.Settings`: `vcs.revision` (short-hashed), `vcs.modified` ("dirty"),
    `vcs.time` (build date).
  - `bi.GoVersion` and `runtime.GOOS/GOARCH` for the platform line.
  - All build-info reads are best-effort: absent fields are simply omitted, so a
    plain `go build` still prints a clean `cowbird dev` line.

  Target output:
  ```
  cowbird 0.7.0
    commit: 3be234c (dirty)
    built:  go1.26 linux/amd64
  ```

### `main.go` restructure

The current `main()` body becomes `runGUI()`; `main()` shrinks to dispatch:

```go
func main() {
    if err := cli.Execute(runGUI); err != nil {
        os.Exit(1)
    }
}

func runGUI() {
    // ... existing main() body verbatim: app.NewWithID, config.Load,
    //     needsSetup logic, openUnlock, setup/connecting windows ...
}
```

No behavioral change to the GUI path — it is the same code, now reached via the
root command's `Run` (FR-002). `log.Fatalf` calls inside `runGUI` stay as-is;
they already exit the process.

### Version source of truth

`FyneApp.toml` (`Version = "0.7.0"`) stays the packaging source the `fyne` tool
reads; it is not readable at runtime. The runtime version is the linker-injected
`cli.version` var. These two must be kept in sync at release time — same class of
hand-sync convention as the `cowbird-user-access.hcl` ↔ live-policy note in
CLAUDE.md. For a plain local build, inject the version directly, e.g.:

```sh
go build -ldflags "-X cowbird/internal/cli.version=0.7.0" -o cowbird .
```

For releases, the GitHub workflow (`.github/workflows/release.yml`) is the
authoritative path: the pushed semver tag (`GITHUB_REF_NAME`, e.g. `v0.7.0`) is
the single source of truth. The Package step strips the leading `v` and feeds
the result to *both* `fyne package --app-version` (packaging metadata) and
`cli.version` via `GOFLAGS=-ldflags=-X=...`. `fyne package` forwards `GOFLAGS`
to the underlying `go build` (verified empirically — it does not set its own
`-ldflags`), so the runtime version and the packaging version are always the
same tag, with nothing to hand-sync. Go's VCS stamping still populates the
commit/date lines independently of the injected ldflags.

## Build Order (risk-first)

1. Add `github.com/spf13/cobra` (`go get`); create `internal/cli` with
   `Execute` + a no-op-friendly root command. Verify bare `cowbird` still opens
   the GUI and `cowbird --help` lists commands.
2. `version.go`: the `version` var, build-info reading, and command. Verify
   `cowbird version` prints and exits 0 with no window.
3. Restructure `main.go` (`runGUI` extraction) — kept last/minimal so the GUI
   path is touched as little as possible.

## Tests (`internal/cli`)

Cobra commands are testable without a GUI (the whole point):

- `version` command writes a non-empty version line to a captured
  `cmd.OutOrStdout()` and returns no error; output contains the `version` var's
  value.
- Unknown subcommand: `root.Execute()` with args `["frobnicate"]` returns a
  non-nil error and does not invoke the `runGUI` callback (assert via a sentinel
  func that flips a bool).
- Bare invocation (no args) invokes the `runGUI` callback exactly once.
- `main.go`/GUI bootstrap remains untested per project precedent (needs Fyne /
  Vault); covered indirectly.

## Notes / follow-ups

- Future data subcommands (`list`, `get`, `generate`) attach to the same root
  via `root.AddCommand(...)`. They will need: a terminal no-echo password prompt
  (`golang.org/x/term`) for unlock, a headless keyring story for Vault
  credentials, and reuse of `config.Load` → `vault.NewVault` →
  `core.InitIdentity`. None of that is built here (see spec Out of Scope).
- `generate` is the cheapest next command since `internal/generate` needs no
  unlock; a natural second leaf after this scaffold lands.
- Consider a release build target that derives `cli.version` from
  `FyneApp.toml` to prevent drift.
