package ui

import (
	"fmt"

	"cowbird/internal/auth"
	"cowbird/internal/config"
	"cowbird/internal/credentials"
	"cowbird/internal/vault"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// NewConnectingWindow builds the returning-user startup window. It attempts to
// connect to Vault using the saved configuration; if the server cannot be
// reached it presents a dialog explaining the failure and offering to edit the
// connection details (which reopens the setup window pre-filled) or retry.
// onConnected is invoked, on the Fyne main thread, with the live Vault once a
// connection succeeds.
func NewConnectingWindow(
	a fyne.App,
	cfg config.Config,
	store credentials.CredentialStore,
	method auth.Method,
	onConnected func(*vault.Vault),
) fyne.Window {
	w := a.NewWindow("Cowbird")
	w.Resize(fyne.NewSize(360, 400))
	w.CenterOnScreen()

	statusLabel := widget.NewLabel("Connecting to Vault…")
	statusLabel.Alignment = fyne.TextAlignCenter
	progress := widget.NewProgressBarInfinite()
	w.SetContent(container.NewPadded(container.NewVBox(progress, statusLabel)))

	// openSetup reopens the setup window so the user can correct the connection
	// details; its onComplete reconnects and continues into the unlock flow.
	openSetup := func() {
		setupW := NewSetupWindow(a, cfg, store, func(newCfg config.Config, newMethod auth.Method, newStore credentials.CredentialStore) error {
			v, err := vault.NewVault(newCfg.Vault, newStore, newMethod)
			if err != nil {
				return err
			}
			fyne.Do(func() { onConnected(v) })
			return nil
		})
		setupW.Show()
		w.Close()
	}

	var attempt func()
	attempt = func() {
		fyne.Do(func() {
			statusLabel.SetText("Connecting to Vault…")
			progress.Show()
		})
		go func() {
			v, err := vault.NewVault(cfg.Vault, store, method)
			fyne.Do(func() {
				if err == nil {
					onConnected(v)
					w.Close()
					return
				}

				progress.Hide()
				var msg string
				if vault.IsUnreachable(err) {
					msg = fmt.Sprintf("Unable to connect to the Vault server at %s.", cfg.Vault.Address)
				} else {
					msg = fmt.Sprintf("Unable to connect to Vault: %v", err)
				}
				statusLabel.SetText(msg)

				detail := widget.NewLabel(msg +
					"\n\nCheck that the server is running and reachable, then retry or edit the connection details.")
				detail.Wrapping = fyne.TextWrapWord

				d := dialog.NewCustomWithoutButtons("Cannot connect to Vault", detail, w)
				retryBtn := widget.NewButtonWithIcon("Retry", theme.ViewRefreshIcon(), func() {
					d.Hide()
					attempt()
				})
				editBtn := widget.NewButtonWithIcon("Edit Connection Details", theme.SettingsIcon(), func() {
					d.Hide()
					openSetup()
				})
				editBtn.Importance = widget.HighImportance
				d.SetButtons([]fyne.CanvasObject{retryBtn, editBtn})
				d.Resize(fyne.NewSize(440, 320))
				d.Show()
			})
		}()
	}

	attempt()
	return w
}
