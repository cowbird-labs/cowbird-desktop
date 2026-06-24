# Releasing cowbird

Releases are produced entirely by CI from a pushed git tag. There is no manual
artifact building or uploading — pushing a `v*` tag is the whole release action.

## Versioning

cowbird uses [semantic versioning](https://semver.org). The git tag is the
single source of truth for a release's version: the tag `vX.Y.Z` produces
version `X.Y.Z`, which is injected into both

- the package metadata (`fyne package --app-version X.Y.Z`), and
- the runtime string reported by `cowbird version`
  (`-ldflags -X cowbird/internal/cli.version=X.Y.Z`).

Both are derived from the tag in the same step (`.github/workflows/release.yml`),
so packaging metadata and the runtime version can never drift.

`FyneApp.toml`'s `Version` is the fallback for unbundled/local builds; keep it
roughly in step with the latest tag, but the tag wins for released artifacts.

## Cutting a release

1. Make sure `main` is green (`go build ./... && go test ./...`) and that
   `CHANGELOG`/release notes are ready (notes are auto-generated from commits,
   but review them).
2. Tag and push:

   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

3. The **Release** workflow triggers on the `v*` tag and:
   - builds a package on a **native runner** per target (Fyne uses CGO, so we do
     not cross-compile) for: `linux-amd64`, `linux-arm64`, `windows-amd64`,
     `darwin-amd64`, `darwin-arm64`;
   - builds with `-trimpath` so the binary does not embed the runner's build
     paths (reproducibility);
   - collects each target into a single archive (`.tar.xz` on Linux, `.zip` on
     Windows/macOS);
   - generates `SHA256SUMS.txt` over all artifacts;
   - creates the GitHub release **as a draft**, uploads every asset to it, then
     flips it to published — the order required for immutable releases (assets
     are sealed at publication, so all must be attached first).

   The publish job is idempotent and retry-safe: re-running reuses an existing
   draft and `--clobber`s assets, so a failed run can simply be re-run.

## Toolchain

CI installs Go via `actions/setup-go` with `go-version-file: go.mod`, so the
`go 1.26` directive in `go.mod` pins the toolchain to the 1.26 series. Bump that
directive (and re-tag) to move the release toolchain.

## Verifying a release

After the workflow finishes:

- Download an artifact and confirm `cowbird version` prints the tagged version,
  the commit, and the build metadata.
- Verify checksums against `SHA256SUMS.txt`:

  ```sh
  sha256sum -c SHA256SUMS.txt
  ```

## What is *not* automated

- Code signing / notarization (macOS) and Authenticode (Windows) are not yet
  wired up; artifacts are unsigned. Tracked for a future release.
- Homebrew / package-manager distribution is not set up.
