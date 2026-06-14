package ui

import (
	"context"
	"fmt"

	"cowbird/internal/core"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// showMainMenu pops up the hamburger menu, anchored just below the menu
// button. New app-level actions (export key, etc.) hang off this menu.
func (m *mainWindow) showMainMenu() {
	menu := fyne.NewMenu("",
		fyne.NewMenuItem("Change Password…", m.showChangePasswordDialog),
		fyne.NewMenuItem("Rotate Key…", m.showRotateKeyDialog),
		fyne.NewMenuItem("Export Recovery Key…", m.showExportKeyDialog),
	)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(m.menuBtn)
	pos.Y += m.menuBtn.Size().Height
	widget.ShowPopUpMenuAtPosition(menu, m.win.Canvas(), pos)
}

// showChangePasswordDialog presents the change-unlock-password form: current,
// new, and confirm fields with an advisory strength meter. The keypair is
// unchanged by this operation, so the current session stays live and the item
// list does not reload.
//
// A custom dialog (not ShowCustomConfirm) is used so validation errors and a
// failed Vault write keep the form open for retry rather than discarding the
// user's typed passwords.
func (m *mainWindow) showChangePasswordDialog() {
	currentEntry := widget.NewPasswordEntry()
	currentEntry.SetPlaceHolder("Current password")
	newEntry := widget.NewPasswordEntry()
	newEntry.SetPlaceHolder("New password")
	confirmEntry := widget.NewPasswordEntry()
	confirmEntry.SetPlaceHolder("Confirm new password")

	strengthLabel := ""
	strengthBar := widget.NewProgressBar()
	strengthBar.TextFormatter = func() string { return strengthLabel }
	newEntry.OnChanged = func(s string) {
		var score float64
		score, strengthLabel = passwordStrength(s)
		strengthBar.SetValue(score)
	}

	errorLabel := widget.NewLabel("")
	changeBtn := widget.NewButton("Change Password", nil)

	content := container.NewVBox(
		widget.NewLabel("Re-wraps your key under a new password.\nYour items are not re-encrypted and stay accessible."),
		widget.NewSeparator(),
		currentEntry,
		newEntry,
		strengthBar,
		confirmEntry,
		errorLabel,
		changeBtn,
	)

	d := dialog.NewCustom("Change Unlock Password", "Cancel", content, m.win)

	changeBtn.OnTapped = func() {
		current := currentEntry.Text
		next := newEntry.Text
		switch {
		case current == "":
			errorLabel.SetText("Enter your current password.")
			return
		case next == "":
			errorLabel.SetText("Enter a new password.")
			return
		case next != confirmEntry.Text:
			errorLabel.SetText("New passwords do not match.")
			return
		case next == current:
			errorLabel.SetText("New password must differ from the current one.")
			return
		}

		errorLabel.SetText("")
		changeBtn.Disable()

		go func() {
			err := core.ChangePassword(context.Background(), m.app.Vault, []byte(current), []byte(next))
			fyne.Do(func() {
				if err != nil {
					errorLabel.SetText(fmt.Sprintf("Error: %v", err))
					changeBtn.Enable()
					return
				}
				d.Hide()
				dialog.ShowInformation("Password changed", "Your unlock password has been changed.", m.win)
			})
		}()
	}

	d.Show()
}

// showRotateKeyDialog presents the key-rotation flow: a compromise-recovery
// warning plus a password confirmation. Rotation re-encrypts every owned item
// under a new keypair and re-distributes shares, so it is gated behind an
// explicit confirmation and may take a while.
//
// Like the change-password dialog this is a custom dialog so errors keep the
// form open for retry — important here because rotation is resumable and a
// retry finishes an interrupted run.
func (m *mainWindow) showRotateKeyDialog() {
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Unlock password")

	errorLabel := widget.NewLabel("")
	rotateBtn := widget.NewButton("Rotate Key", nil)

	warning := widget.NewLabel(
		"Rotate your key only if it may be compromised.\n\n" +
			"Every item you own is re-encrypted under a new key and recipients\n" +
			"are re-keyed automatically. Close any other open Cowbird sessions\n" +
			"first. Items others shared with you must be re-shared by their\n" +
			"owners afterward.\n\n" +
			"Enter your unlock password to continue.")

	content := container.NewVBox(
		warning,
		widget.NewSeparator(),
		passwordEntry,
		errorLabel,
		rotateBtn,
	)

	d := dialog.NewCustom("Rotate Key", "Cancel", content, m.win)

	rotateBtn.OnTapped = func() {
		password := passwordEntry.Text
		if password == "" {
			errorLabel.SetText("Enter your unlock password.")
			return
		}

		errorLabel.SetText("Rotating… this may take a moment.")
		rotateBtn.Disable()

		go func() {
			err := core.RotateKey(context.Background(), m.app, []byte(password))
			fyne.Do(func() {
				if err != nil {
					errorLabel.SetText(fmt.Sprintf("Error: %v", err))
					rotateBtn.Enable()
					return
				}
				d.Hide()
				dialog.ShowInformation("Key rotated", "Your key has been rotated and your items re-secured.", m.win)
				m.reload()
			})
		}()
	}

	d.Show()
}

// showExportKeyDialog presents the recovery-key export flow: an authorization
// (unlock password) plus a separate passphrase that protects the file. On
// success it opens a file-save dialog and writes the passphrase-encrypted key.
// Custom dialog so auth/validation errors keep the form open for retry.
func (m *mainWindow) showExportKeyDialog() {
	unlockEntry := widget.NewPasswordEntry()
	unlockEntry.SetPlaceHolder("Current unlock password")
	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("Export passphrase")
	confirmEntry := widget.NewPasswordEntry()
	confirmEntry.SetPlaceHolder("Confirm export passphrase")

	strengthLabel := ""
	strengthBar := widget.NewProgressBar()
	strengthBar.TextFormatter = func() string { return strengthLabel }
	passEntry.OnChanged = func(s string) {
		var score float64
		score, strengthLabel = passwordStrength(s)
		strengthBar.SetValue(score)
	}

	errorLabel := widget.NewLabel("")
	exportBtn := widget.NewButton("Export…", nil)

	content := container.NewVBox(
		widget.NewLabel(
			"Save your private key to a passphrase-protected file. This is the\n"+
				"ONLY way to recover access if you lose your device — there is no\n"+
				"operator reset. Store the file somewhere safe and offline."),
		widget.NewSeparator(),
		unlockEntry,
		passEntry,
		strengthBar,
		confirmEntry,
		errorLabel,
		exportBtn,
	)

	d := dialog.NewCustom("Export Recovery Key", "Cancel", content, m.win)

	exportBtn.OnTapped = func() {
		unlock := unlockEntry.Text
		pass := passEntry.Text
		switch {
		case unlock == "":
			errorLabel.SetText("Enter your current unlock password.")
			return
		case pass == "":
			errorLabel.SetText("Enter an export passphrase.")
			return
		case pass != confirmEntry.Text:
			errorLabel.SetText("Export passphrases do not match.")
			return
		}

		errorLabel.SetText("")
		exportBtn.Disable()

		go func() {
			data, err := core.ExportIdentity(context.Background(), m.app, []byte(unlock), []byte(pass))
			fyne.Do(func() {
				if err != nil {
					errorLabel.SetText(fmt.Sprintf("Error: %v", err))
					exportBtn.Enable()
					return
				}
				d.Hide()
				m.saveRecoveryFile(data)
			})
		}()
	}

	d.Show()
}

// saveRecoveryFile prompts for a location and writes the recovery bytes there.
// Success is reported only once the file is written and closed without error.
func (m *mainWindow) saveRecoveryFile(data []byte) {
	save := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if w == nil {
			return // user cancelled
		}
		_, werr := w.Write(data)
		cerr := w.Close()
		if werr != nil {
			dialog.ShowError(fmt.Errorf("writing recovery file: %w", werr), m.win)
			return
		}
		if cerr != nil {
			dialog.ShowError(fmt.Errorf("closing recovery file: %w", cerr), m.win)
			return
		}
		dialog.ShowInformation("Recovery key exported",
			"Your recovery key was saved. Keep it somewhere safe and offline.", m.win)
	}, m.win)
	save.SetFileName("cowbird-recovery.json")
	save.Show()
}
