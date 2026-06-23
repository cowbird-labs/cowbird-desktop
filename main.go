package main

import (
	"cowbird/internal/auth"
	"cowbird/internal/cli"
	"cowbird/internal/config"
	"cowbird/internal/core"
	"cowbird/internal/credentials"
	"cowbird/internal/ui"
	"cowbird/internal/vault"
	_ "embed"
	"log"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/99designs/keyring"
)

// appIconPNG is embedded so the window icon renders even when the binary is run
// unbundled (go run/go build), where Fyne would otherwise have no app icon.
//
//go:embed assets/icons/cowbirdicon180.png
var appIconPNG []byte

// trayIconPNG is the round white badge used specifically for the system tray
// (distinct from the window icon), embedded so it is available unbundled.
//
//go:embed assets/icons/cowbird-tray.png
var trayIconPNG []byte

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
	a.SetIcon(fyne.NewStaticResource("cowbird.png", appIconPNG))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	// The system tray must be installed before the run loop starts (see
	// ui.Tray); the resulting controller is attached to each window as the
	// startup flow advances. nil when the preference is off or the platform has
	// no tray, in which case Tray's methods are no-ops.
	var tray *ui.Tray
	if cfg.UI.SystemTray {
		tray = ui.NewTray(a, fyne.NewStaticResource("cowbird-tray.png", trayIconPNG))
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
			mainW := ui.NewMainWindow(a, coreApp, tray)
			mainW.Show()
		})
		tray.Attach(unlockW)
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
		tray.Attach(w)
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
	tray.Attach(w)
	w.ShowAndRun()
}
