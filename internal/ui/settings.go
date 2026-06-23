package ui

import (
	"fmt"

	"cowbird/internal/auth"
	"cowbird/internal/config"
	"cowbird/internal/core"
	"cowbird/internal/credentials"
	"cowbird/internal/vault"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showSettingsDialog presents the application settings. For now it holds only
// the Vault server connection details; future sections (auto-lock, clipboard
// clearing, etc.) hang off the same dialog as additional blocks in the VBox
// assembled below.
//
// Settings are read fresh from disk so the dialog reflects what is persisted,
// not a stale in-memory copy. The credential store is opened lazily so editing
// the connection can pre-fill the existing credentials.
func (m *mainWindow) showSettingsDialog() {
	cfg, err := config.Load()
	if err != nil {
		dialog.ShowError(fmt.Errorf("loading settings: %w", err), m.win)
		return
	}

	// sections collects each settings block; they are stacked with separators
	// between them so new sections can be appended without reflowing the rest.
	sections := []fyne.CanvasObject{m.buildServerSection(cfg)}

	content := container.NewVBox()
	for i, s := range sections {
		if i > 0 {
			content.Add(widget.NewSeparator())
		}
		content.Add(s)
	}

	var d dialog.Dialog
	// The settings content is read-only (labels + a button) and not focusable,
	// so a hidden key-catcher carries Escape — Fyne routes keys only to the
	// focused widget and its dialogs do not dismiss on Escape themselves. Enter
	// also closes, matching the single "Close" button.
	dismiss := func() { d.Hide() }
	catcher := newKeyCatcher(dismiss, dismiss)
	d = dialog.NewCustom("Settings", "Close", container.NewStack(content, catcher), m.win)
	d.Show()
	m.win.Canvas().Focus(catcher)
}

// buildServerSection renders the read-only Vault connection summary plus an
// "Edit Connection Details…" button. Editing the connection re-authenticates
// and reconnects, which invalidates the current session, so it is delegated to
// the dedicated setup window rather than edited inline here.
func (m *mainWindow) buildServerSection(cfg config.Config) fyne.CanvasObject {
	header := widget.NewLabelWithStyle("Server", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	form := widget.NewForm(
		widget.NewFormItem("Vault Address", widget.NewLabel(orPlaceholder(cfg.Vault.Address))),
		widget.NewFormItem("Mount Path", widget.NewLabel(orPlaceholder(cfg.Vault.MountPath))),
		widget.NewFormItem("Auth Method", widget.NewLabel(orPlaceholder(cfg.Vault.AuthMethod))),
	)

	editBtn := widget.NewButtonWithIcon("Edit Connection Details…", theme.SettingsIcon(), func() {
		m.editConnection(cfg)
	})

	return container.NewVBox(header, form, editBtn)
}

// editConnection opens the setup window pre-filled with the current connection
// so the user can change the address, mount, auth method, or credentials.
// Changing the connection means a new Vault session, so on success the old
// connection is torn down and the app re-enters the unlock flow with the new
// one, replacing the current main window.
func (m *mainWindow) editConnection(cfg config.Config) {
	a := fyne.CurrentApp()

	store, err := credentials.NewStore("cowbird")
	if err != nil {
		dialog.ShowError(fmt.Errorf("opening credential store: %w", err), m.win)
		return
	}

	// onComplete runs on the setup window's connect goroutine; the new Vault
	// connection is built here, but all window work must return to the Fyne
	// main thread.
	setupW := NewSetupWindow(a, cfg, store, func(newCfg config.Config, method auth.Method, newStore credentials.CredentialStore) error {
		v, err := vault.NewVault(newCfg.Vault, newStore, method)
		if err != nil {
			return err
		}
		fyne.Do(func() {
			m.app.Vault.Close() // stop the old token-renewal loop
			NewUnlockWindow(a, v, func(coreApp *core.App) {
				NewMainWindow(a, coreApp).Show()
			}).Show()
			m.win.Close()
		})
		return nil
	})
	setupW.Show()
}

// orPlaceholder returns s, or an em dash when s is empty, so blank settings
// read as "not set" rather than a collapsed, invisible label.
func orPlaceholder(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
