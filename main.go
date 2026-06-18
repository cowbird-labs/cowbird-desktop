package main

import (
	"cowbird/internal/auth"
	"cowbird/internal/cli"
	"cowbird/internal/config"
	"cowbird/internal/core"
	"cowbird/internal/credentials"
	"cowbird/internal/ui"
	"cowbird/internal/vault"
	"log"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/99designs/keyring"
)

func main() {
	// Dispatch CLI subcommands; with no subcommand this runs runGUI.
	if err := cli.Execute(runGUI); err != nil {
		os.Exit(1)
	}
}

// runGUI is the GUI bootstrap: setup → connect → unlock → main window. It is
// the root command's no-argument path, unchanged from the previous main().
func runGUI() {
	a := app.NewWithID("co.avitac.cowbird")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	needsSetup := cfg.Vault.Address == "" || cfg.Vault.AuthMethod == ""

	if !needsSetup {
		// Config exists — check that credentials are actually present.
		store, err := credentials.NewStore("cowbird")
		if err != nil {
			log.Fatalf("error opening credential store: %v", err)
		}
		method := auth.ByName(cfg.Vault.AuthMethod)
		if method == nil {
			log.Fatalf("unknown auth method %q in config — delete config and re-run setup", cfg.Vault.AuthMethod)
		}
		if len(method.Fields()) > 0 {
			if _, err := store.Get(method.Fields()[0].Key); err == keyring.ErrKeyNotFound {
				needsSetup = true
			}
		}
	}

	// openUnlock opens the unlock window for a live Vault. Must run on the Fyne
	// main thread.
	openUnlock := func(v *vault.Vault) {
		unlockW := ui.NewUnlockWindow(a, v, func(coreApp *core.App) {
			mainW := ui.NewMainWindow(a, coreApp)
			mainW.Show()
		})
		unlockW.Show()
	}

	if needsSetup {
		// This callback runs on the setup window's connect goroutine: the
		// Vault connection stays here, but window work must go back to the
		// Fyne main thread.
		w := ui.NewSetupWindow(a, cfg, nil, func(cfg config.Config, method auth.Method, store credentials.CredentialStore) error {
			v, err := vault.NewVault(cfg.Vault, store, method)
			if err != nil {
				return err
			}
			fyne.Do(func() { openUnlock(v) })
			return nil
		})
		w.ShowAndRun()
		return
	}

	store, err := credentials.NewStore("cowbird")
	if err != nil {
		log.Fatalf("error opening credential store: %v", err)
	}

	method := auth.ByName(cfg.Vault.AuthMethod)

	// NewConnectingWindow attempts the connection itself; if the server is
	// unreachable it shows a dialog offering to edit the connection details
	// rather than crashing.
	w := ui.NewConnectingWindow(a, cfg, store, method, openUnlock)
	w.ShowAndRun()
}
