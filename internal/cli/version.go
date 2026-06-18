package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is the build-time version string, overridden via linker flags:
//
//	go build -ldflags "-X cowbird/internal/cli.version=0.7.0"
//
// It defaults to "dev" for a plain `go build`. FyneApp.toml remains the source
// of truth for packaging metadata; this var is the runtime version and must be
// kept in sync at release time.
var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		// Reads no config, keyring, or Vault — pure local build metadata.
		Run: func(cmd *cobra.Command, args []string) {
			printVersion(cmd.OutOrStdout())
		},
	}
}

// printVersion writes the headline version plus any available build metadata.
// Every build-info read is best-effort: absent fields are omitted, so a plain
// `go build` still prints a clean "cowbird dev" line.
func printVersion(w io.Writer) {
	fmt.Fprintf(w, "cowbird %s\n", version)

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	var revision, modified, buildTime string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		case "vcs.time":
			buildTime = s.Value
		}
	}

	if revision != "" {
		short := revision
		if len(short) > 7 {
			short = short[:7]
		}
		if modified == "true" {
			short += " (dirty)"
		}
		fmt.Fprintf(w, "  commit: %s\n", short)
	}
	if buildTime != "" {
		fmt.Fprintf(w, "  date:   %s\n", buildTime)
	}

	goVersion := bi.GoVersion
	if goVersion == "" {
		goVersion = runtime.Version()
	}
	fmt.Fprintf(w, "  built:  %s %s/%s\n", goVersion, runtime.GOOS, runtime.GOARCH)
}
