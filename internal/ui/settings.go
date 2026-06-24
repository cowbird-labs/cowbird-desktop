package ui

import (
	"fmt"
	"strconv"
	"strings"

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

// showSettingsDialog presents the application settings: a General section
// (system tray, auto-lock, and clipboard clearing) and the Vault server
// connection details. Future sections hang off the same dialog as additional
// blocks in the VBox assembled below.
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

	var d dialog.Dialog
	dismiss := func() { d.Hide() }

	// Each interactive control forwards Escape (escapableCheck/escapableButton),
	// so Escape dismisses the dialog whenever one of them holds focus. An earlier
	// version instead stacked an invisible full-size key-catcher over the content,
	// which blocked taps on the controls — avoided here.
	content := container.NewVBox(
		m.buildGeneralSection(cfg, dismiss),
		widget.NewSeparator(),
		m.buildServerSection(cfg, dismiss),
	)

	d = dialog.NewCustom("Settings", "Close", content, m.win)

	// Escape fallback for the moment after Show when no control yet holds focus
	// (focused controls forward Escape themselves). Mirrors the generator dialog;
	// restore any prior handler when the dialog closes. onTypedKey only fires
	// when nothing is focused, so it cannot shadow the controls.
	prevKey := m.win.Canvas().OnTypedKey()
	m.win.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyEscape {
			dismiss()
			return
		}
		if prevKey != nil {
			prevKey(ev)
		}
	})
	d.SetOnClosed(func() { m.win.Canvas().SetOnTypedKey(prevKey) })

	d.Show()
}

// buildGeneralSection renders the general application preferences: the
// system-tray toggle, auto-lock, and clipboard clearing. dismiss is wired to
// each control's Escape handler. Each control persists its own change
// immediately and (for auto-lock and clipboard clearing) applies it to the live
// session.
func (m *mainWindow) buildGeneralSection(cfg config.Config, dismiss func()) fyne.CanvasObject {
	header := widget.NewLabelWithStyle("General", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	trayCheck := newEscapableCheck("Keep running in the system tray when the window is closed", dismiss)
	trayCheck.SetChecked(cfg.UI.SystemTray)
	// Set OnChanged after SetChecked so the initial state does not trigger a save.
	trayCheck.OnChanged = func(enabled bool) {
		if err := saveUISetting(func(ui *config.UI) { ui.SystemTray = enabled }); err != nil {
			dialog.ShowError(fmt.Errorf("saving setting: %w", err), m.win)
			// Revert the visual state so it reflects what is actually
			// persisted. Set the field directly rather than via SetChecked,
			// which would re-fire OnChanged and recurse.
			trayCheck.Checked = !enabled
			trayCheck.Refresh()
		}
	}

	// The tray cannot be started or removed reliably mid-session in Fyne (it
	// runs once for the app's lifetime), so this preference is applied at the
	// next launch rather than live.
	note := widget.NewLabel("Takes effect after restarting Cowbird.")
	note.Importance = widget.LowImportance
	note.TextStyle = fyne.TextStyle{Italic: true}

	autoLock := m.buildAutoLockControl(cfg, dismiss)
	clipboard := m.buildClipboardClearControl(cfg, dismiss)

	return container.NewVBox(
		header,
		trayCheck,
		note,
		widget.NewSeparator(),
		autoLock,
		widget.NewSeparator(),
		clipboard,
	)
}

// buildAutoLockControl renders the auto-lock toggle and its inactivity timeout
// (in minutes). Changes persist immediately and re-arm the live inactivity timer.
func (m *mainWindow) buildAutoLockControl(cfg config.Config, dismiss func()) fyne.CanvasObject {
	check := newEscapableCheck("Lock automatically after a period of inactivity", dismiss)

	minutes := newEscapableTextEntry(dismiss)
	minutes.SetText(strconv.Itoa(cfg.UI.AutoLockMinutes))
	minutes.Validator = positiveIntValidator("minutes")
	unit := container.NewBorder(nil, nil, widget.NewLabel("Lock after"), widget.NewLabel("minutes"), minutes)

	apply := func() {
		enabled := check.Checked
		if enabled {
			minutes.Enable()
		} else {
			minutes.Disable()
		}
		// When enabled the value must be a positive integer; otherwise hold the
		// last persisted setting and let the validator flag the bad input.
		n, err := strconv.Atoi(strings.TrimSpace(minutes.Text))
		if enabled && (err != nil || n <= 0) {
			return
		}
		if saveErr := saveUISetting(func(ui *config.UI) {
			ui.AutoLock = enabled
			if n > 0 {
				ui.AutoLockMinutes = n
			}
		}); saveErr != nil {
			dialog.ShowError(fmt.Errorf("saving setting: %w", saveErr), m.win)
			return
		}
		m.autoLockDur = autoLockDuration(config.UI{AutoLock: enabled, AutoLockMinutes: n})
		m.startAutoLock()
	}

	check.SetChecked(cfg.UI.AutoLock)
	if cfg.UI.AutoLock {
		minutes.Enable()
	} else {
		minutes.Disable()
	}
	// Wire callbacks after the initial SetChecked/SetText so they don't fire a save.
	check.OnChanged = func(bool) { apply() }
	minutes.OnChanged = func(string) { apply() }

	return container.NewVBox(check, unit)
}

// buildClipboardClearControl renders the clipboard-clearing toggle and its delay
// (in seconds). Changes persist immediately and apply to the live session.
func (m *mainWindow) buildClipboardClearControl(cfg config.Config, dismiss func()) fyne.CanvasObject {
	check := newEscapableCheck("Clear the clipboard after copying a value", dismiss)

	seconds := newEscapableTextEntry(dismiss)
	seconds.SetText(strconv.Itoa(cfg.UI.ClipboardClearSeconds))
	seconds.Validator = positiveIntValidator("seconds")
	unit := container.NewBorder(nil, nil, widget.NewLabel("Clear after"), widget.NewLabel("seconds"), seconds)

	apply := func() {
		enabled := check.Checked
		if enabled {
			seconds.Enable()
		} else {
			seconds.Disable()
		}
		n, err := strconv.Atoi(strings.TrimSpace(seconds.Text))
		if enabled && (err != nil || n <= 0) {
			return
		}
		if saveErr := saveUISetting(func(ui *config.UI) {
			ui.ClipboardClear = enabled
			if n > 0 {
				ui.ClipboardClearSeconds = n
			}
		}); saveErr != nil {
			dialog.ShowError(fmt.Errorf("saving setting: %w", saveErr), m.win)
			return
		}
		m.clipClearDur = clipboardClearDuration(config.UI{ClipboardClear: enabled, ClipboardClearSeconds: n})
	}

	check.SetChecked(cfg.UI.ClipboardClear)
	if cfg.UI.ClipboardClear {
		seconds.Enable()
	} else {
		seconds.Disable()
	}
	check.OnChanged = func(bool) { apply() }
	seconds.OnChanged = func(string) { apply() }

	return container.NewVBox(check, unit)
}

// positiveIntValidator returns a validator that accepts only a positive integer,
// naming the unit in the error message.
func positiveIntValidator(unit string) func(string) error {
	return func(s string) error {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n <= 0 {
			return fmt.Errorf("enter a whole number of %s greater than 0", unit)
		}
		return nil
	}
}

// saveUISetting loads the on-disk config, applies mutate to the UI section, and
// writes it back, preserving the rest of the config.
func saveUISetting(mutate func(*config.UI)) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	mutate(&cfg.UI)
	return config.Save(cfg)
}

// buildServerSection renders the read-only Vault connection summary plus an
// "Edit Connection Details…" button. Editing the connection re-authenticates
// and reconnects, which invalidates the current session, so it is delegated to
// the dedicated setup window rather than edited inline here.
func (m *mainWindow) buildServerSection(cfg config.Config, dismiss func()) fyne.CanvasObject {
	header := widget.NewLabelWithStyle("Server", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	form := widget.NewForm(
		widget.NewFormItem("Vault Address", widget.NewLabel(orPlaceholder(cfg.Vault.Address))),
		widget.NewFormItem("Mount Path", widget.NewLabel(orPlaceholder(cfg.Vault.MountPath))),
		widget.NewFormItem("Auth Method", widget.NewLabel(orPlaceholder(cfg.Vault.AuthMethod))),
	)

	editBtn := newEscapableButton("Edit Connection Details…", theme.SettingsIcon(), func() {
		m.editConnection(cfg)
	}, dismiss)

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
			unlockW := NewUnlockWindow(a, v, func(coreApp *core.App) {
				NewMainWindow(a, coreApp, m.tray).Show()
			})
			m.tray.Attach(unlockW) // keep close-to-tray during the re-unlock
			unlockW.Show()
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
