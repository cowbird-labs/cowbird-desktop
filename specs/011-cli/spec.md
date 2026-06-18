# Feature Specification: CLI Command Scaffold

**Feature Branch**: `011-cli`
**Status**: Draft
**Created**: 2026-06-18

## Summary

Cowbird is a single binary that, today, always launches the Fyne GUI. This
feature introduces a command-line surface so the same binary can run
non-graphical subcommands — starting with `cowbird version` — without ever
opening a window. The work establishes a [Cobra](https://github.com/spf13/cobra)
command tree as the program's entry point, preserves today's behavior (running
`cowbird` with no arguments still opens the GUI), and lands one real subcommand
(`version`) to prove the path end-to-end. Data-bearing commands (list, get,
generate, etc.) are explicitly future work; this spec is the scaffold plus the
first leaf.

The architecture already anticipates this: core logic (`core`, `vault`,
`crypto`, `sharing`, `items`, `generate`) is decoupled from `ui` precisely so a
CLI can reuse it (see CLAUDE.md, and the `generate` spec's note that "a future
CLI `generate` subcommand can call `internal/generate` directly"). This feature
builds the door those packages were waiting behind.

## User Scenarios

### User Story 1 — Run a subcommand without the GUI (Priority: P1)

A user (or a script, or a packager) runs `cowbird version` in a terminal and
gets a version string back immediately, with no window appearing and no Vault or
config access required.

**Why this priority**: This is the entire point of the feature — proving the
binary can act as a CLI tool. `version` is the smallest possible such command
(no secrets, no network, no unlock), making it the right first leaf.

**Acceptance Scenarios**:

1. **Given** the cowbird binary, **When** the user runs `cowbird version`,
   **Then** a human-readable version string is printed to stdout and the process
   exits 0 without any GUI window being created.
2. **Given** the cowbird binary, **When** the user runs `cowbird version` on a
   headless machine with no display, **Then** it still succeeds (it touches no
   GUI, config, or Vault code path).
3. **Given** a build produced with version metadata injected, **When** the user
   runs `cowbird version`, **Then** the output reflects that build's version and
   available build info (commit, dirty state, Go version, platform).

### User Story 2 — GUI remains the default (Priority: P1)

An existing user double-clicks the app icon or runs `cowbird` with no arguments
and the GUI opens exactly as before.

**Why this priority**: Introducing a CLI must not regress the primary, graphical
use case or the way desktop launchers invoke the binary (no arguments).

**Acceptance Scenarios**:

1. **Given** the cowbird binary, **When** it is run with no arguments, **Then**
   the existing GUI startup flow runs unchanged (setup → connect → unlock →
   main window, per current behavior).
2. **Given** a desktop launcher that invokes the binary with no arguments,
   **When** the user opens the app, **Then** behavior is identical to today.

### User Story 3 — Discoverable command surface (Priority: P2)

A user who doesn't know what the binary can do runs `cowbird help` (or
`cowbird --help`) and sees the available subcommands.

**Acceptance Scenarios**:

1. **Given** the cowbird binary, **When** the user runs `cowbird help` or
   `cowbird --help`, **Then** usage text lists the available subcommands
   (currently `version`, plus Cobra's built-in `help` and `completion`).
2. **Given** an unrecognized subcommand, **When** the user runs e.g.
   `cowbird frobnicate`, **Then** the program prints an error naming the unknown
   command and exits non-zero, without launching the GUI.

## Requirements

### Functional Requirements

- **FR-001**: The binary MUST use a Cobra command tree as its entry point, with
  subcommands dispatched from `os.Args`.
- **FR-002**: Running the binary with no subcommand (no arguments) MUST launch
  the existing GUI flow, unchanged from current behavior.
- **FR-003**: The system MUST provide a `version` subcommand that prints version
  information to stdout and exits 0 without creating any GUI window.
- **FR-004**: The `version` command MUST NOT require, read, or initialize
  config, the OS keyring, a Vault connection, or an unlocked identity.
- **FR-005**: The runtime version string MUST be derivable at build time via Go
  linker flags (`-ldflags -X`), defaulting to a sensible placeholder (e.g.
  `dev`) for plain `go build`. The command MUST additionally surface build
  metadata available from `runtime/debug.ReadBuildInfo()` (VCS revision, dirty
  flag, build date) and the Go version / target platform when present.
- **FR-006**: An unknown subcommand MUST produce an error and a non-zero exit
  code, and MUST NOT fall through to launching the GUI.
- **FR-007**: The command surface MUST be discoverable via `help` / `--help`,
  listing available subcommands.
- **FR-008**: The CLI command tree SHOULD live in a UI-independent package so
  that future subcommands can depend on `core`/`vault`/`crypto`/`generate`
  without the GUI being forced to import command-layer code, and so command
  definitions stay out of `main.go`.

### Key Entities

- **Root command**: the top-level Cobra command; with no subcommand it runs the
  GUI bootstrap (provided by `main`).
- **Subcommand**: a leaf command (initially `version`) that runs to completion
  and exits without GUI involvement.
- **Version info**: the version string (linker-injected) plus build metadata
  read from the embedded build info.

## Assumptions

- `github.com/spf13/cobra` is an acceptable new dependency. Viper (already a
  dependency) is from the same ecosystem and integrates cleanly, though this
  spec does not wire Cobra↔Viper flag binding (no flags beyond Cobra built-ins
  yet).
- The binary remains a single artifact that links the GUI/OpenGL stack even when
  used purely as a CLI; CLI subcommands simply never call into it. A slimmer
  CLI-only build is not a goal here.
- `FyneApp.toml` remains the source of truth for *packaging* metadata (the
  `fyne` tool reads it). The runtime version is injected separately at build
  time; keeping the two in sync is a release-process responsibility, noted like
  the policy `.hcl` ↔ live-policy sync convention in CLAUDE.md.
- Desktop launchers invoke the binary with no arguments, so FR-002 preserves
  their behavior.

## Out of Scope

- Any subcommand that requires an unlocked identity or Vault access (`list`,
  `get`, `share`, etc.). These are future work and will need a terminal-based
  unlock flow (no-echo password prompt) and a headless keyring story — neither
  is designed here.
- A `generate` subcommand. Although `internal/generate` is CLI-ready, wiring it
  is deferred to keep this scaffold minimal.
- Cobra↔Viper flag/config binding and persistent global flags.
- Shell-completion polish beyond Cobra's built-in `completion` command.
- Mobile builds (the CLI is a desktop/server concern).
- A slimmer build that excludes the GUI/OpenGL dependencies.
