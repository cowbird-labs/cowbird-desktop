// Package cli is the command-line entry point for cowbird. It builds a Cobra
// command tree and dispatches subcommands. Running the binary with no
// subcommand falls through to the GUI bootstrap supplied by main, so desktop
// launchers (which invoke the binary with no arguments) are unaffected.
package cli

import (
	"github.com/spf13/cobra"
)

// Execute builds the root command and runs it. runGUI is the existing GUI
// bootstrap, invoked when no subcommand is given. It returns an error on an
// unknown command or a subcommand failure; main maps that to a non-zero exit.
func Execute(runGUI func()) error {
	return newRootCmd(runGUI).Execute()
}

// newRootCmd constructs the root command. Separated from Execute so tests can
// drive it directly (set args, capture output).
func newRootCmd(runGUI func()) *cobra.Command {
	root := &cobra.Command{
		Use:   "cowbird",
		Short: "Cowbird password manager",
		Long: "Cowbird is a password manager backed by HashiCorp Vault.\n\n" +
			"Run with no arguments to open the graphical interface.",
		// No subcommand → existing GUI flow.
		Run: func(cmd *cobra.Command, args []string) { runGUI() },
		// An unknown-command error is self-explanatory; don't also dump usage.
		SilenceUsage: true,
	}
	root.AddCommand(newVersionCmd())
	return root
}
