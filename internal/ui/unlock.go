package ui

import (
	"context"
	"errors"
	"fmt"
	"io"

	"cowbird/internal/auth"
	"cowbird/internal/config"
	"cowbird/internal/core"
	"cowbird/internal/credentials"
	"cowbird/internal/sharing"
	"cowbird/internal/vault"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// UnlockDoneFunc is called after the identity is successfully created or
// unlocked, with the fully initialised App.
type UnlockDoneFunc func(app *core.App)

// NewUnlockWindow creates the identity unlock (or first-run setup) window.
//
// It first checks whether an identity already exists in Vault and presents
// either a "set password" form (first run, with confirmation field) or an
// "enter password" form (returning user). From either form the user can switch
// to the import form to restore their key from a recovery file, or open the
// connection settings to point Cowbird at a different Vault. tray is re-attached
// to any window this one spawns so close-to-tray survives a reconnect; it is
// nil-safe.
func NewUnlockWindow(a fyne.App, v *vault.Vault, tray *Tray, onUnlock UnlockDoneFunc) fyne.Window {
	w := a.NewWindow("Cowbird")
	w.Resize(fyne.NewSize(360, 300))
	w.CenterOnScreen()

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Password")
	confirmEntry := widget.NewPasswordEntry()
	confirmEntry.SetPlaceHolder("Confirm password")

	strengthLabel := ""
	strengthBar := widget.NewProgressBar()
	strengthBar.TextFormatter = func() string { return strengthLabel }

	statusLabel := widget.NewLabel("")
	submitBtn := widget.NewButton("Please wait…", nil)
	submitBtn.Disable()

	// body is a single-slot container; we swap its content after the first-run
	// check completes and when toggling to/from the import form.
	body := container.NewMax(container.NewCenter(widget.NewLabel("Connecting to Vault…")))
	w.SetContent(container.NewPadded(body))

	setStatus := func(msg string) {
		fyne.Do(func() { statusLabel.SetText(msg) })
	}

	var isFirstRun bool

	submitBtn.OnTapped = func() {
		password := []byte(passwordEntry.Text)
		if isFirstRun && passwordEntry.Text != confirmEntry.Text {
			statusLabel.SetText("Passwords do not match")
			return
		}

		submitBtn.Disable()
		statusLabel.SetText("Please wait…")

		go func() {
			defer fyne.Do(func() { submitBtn.Enable() })

			id, err := core.InitIdentity(context.Background(), v, password)
			if err != nil {
				setStatus(fmt.Sprintf("Error: %v", err))
				return
			}

			app := core.NewApp(v, id)
			fyne.Do(func() {
				onUnlock(app)
				w.Close()
			})
		}()
	}

	// passwordForm holds the set/enter-password form so the import form's "Back"
	// action can return to it. It is assigned once the first-run check completes.
	var passwordForm fyne.CanvasObject

	buildImportForm := func() fyne.CanvasObject {
		var fileData []byte
		fileLabel := widget.NewLabel("No file chosen")
		chooseBtn := widget.NewButton("Choose file…", func() {
			dialog.ShowFileOpen(func(r fyne.URIReadCloser, err error) {
				if err != nil {
					dialog.ShowError(err, w)
					return
				}
				if r == nil {
					return // cancelled
				}
				defer r.Close()
				data, err := io.ReadAll(r)
				if err != nil {
					dialog.ShowError(fmt.Errorf("reading recovery file: %w", err), w)
					return
				}
				fileData = data
				fileLabel.SetText(r.URI().Name())
			}, w)
		})

		passphraseEntry := widget.NewPasswordEntry()
		passphraseEntry.SetPlaceHolder("Export passphrase")
		newPwEntry := widget.NewPasswordEntry()
		newPwEntry.SetPlaceHolder("New unlock password")
		confirmPwEntry := widget.NewPasswordEntry()
		confirmPwEntry.SetPlaceHolder("Confirm new unlock password")

		impStrengthLabel := ""
		impStrengthBar := widget.NewProgressBar()
		impStrengthBar.TextFormatter = func() string { return impStrengthLabel }
		newPwEntry.OnChanged = func(s string) {
			var score float64
			score, impStrengthLabel = passwordStrength(s)
			impStrengthBar.SetValue(score)
		}

		importStatus := widget.NewLabel("")
		importBtn := widget.NewButton("Import", nil)
		backBtn := widget.NewButton("Back", func() {
			body.Objects = []fyne.CanvasObject{passwordForm}
			body.Refresh()
		})

		var runImport func(force bool)
		runImport = func(force bool) {
			switch {
			case len(fileData) == 0:
				importStatus.SetText("Choose a recovery file.")
				return
			case passphraseEntry.Text == "":
				importStatus.SetText("Enter the export passphrase.")
				return
			case newPwEntry.Text == "":
				importStatus.SetText("Enter a new unlock password.")
				return
			case newPwEntry.Text != confirmPwEntry.Text:
				importStatus.SetText("New passwords do not match.")
				return
			}

			data := fileData
			passphrase := []byte(passphraseEntry.Text)
			newPw := []byte(newPwEntry.Text)

			importBtn.Disable()
			importStatus.SetText("Importing…")
			go func() {
				id, err := core.ImportIdentity(context.Background(), v, data, passphrase, newPw, force)
				fyne.Do(func() {
					if errors.Is(err, core.ErrIdentityMismatch) {
						importBtn.Enable()
						importStatus.SetText("")
						dialog.ShowConfirm("Different identity",
							"This recovery file is for a different key than the one on record. "+
								"Importing it will make your existing items unreadable. Continue?",
							func(ok bool) {
								if ok {
									runImport(true)
								}
							}, w)
						return
					}
					if err != nil {
						importStatus.SetText(fmt.Sprintf("Error: %v", err))
						importBtn.Enable()
						return
					}
					onUnlock(core.NewApp(v, id))
					w.Close()
				})
			}()
		}
		importBtn.OnTapped = func() { runImport(false) }

		return container.NewVBox(
			widget.NewLabel("Restore your key from a recovery file.\nYou will set a new unlock password."),
			widget.NewSeparator(),
			container.NewBorder(nil, nil, nil, chooseBtn, fileLabel),
			passphraseEntry,
			newPwEntry,
			impStrengthBar,
			confirmPwEntry,
			importBtn,
			importStatus,
			backBtn,
		)
	}

	gotoImportBtn := widget.NewButton("Import recovery key", func() {
		body.Objects = []fyne.CanvasObject{buildImportForm()}
		body.Refresh()
	})

	// openSetup reopens the setup window pre-filled with the saved config so the
	// user can point Cowbird at a different Vault (or fix the current one) before
	// unlocking. On success it builds the new connection and returns to a fresh
	// unlock window; the config and credential store are loaded here rather than
	// threaded in, matching connect.go's reconnect path.
	openSetup := func() {
		cfg, err := config.Load()
		if err != nil {
			dialog.ShowError(fmt.Errorf("loading settings: %w", err), w)
			return
		}
		store, err := credentials.NewStore("cowbird")
		if err != nil {
			dialog.ShowError(fmt.Errorf("opening credential store: %w", err), w)
			return
		}
		setupW := NewSetupWindow(a, cfg, store, func(newCfg config.Config, newMethod auth.Method, newStore credentials.CredentialStore) error {
			nv, err := vault.NewVault(newCfg.Vault, newStore, newMethod)
			if err != nil {
				return err
			}
			fyne.Do(func() {
				nu := NewUnlockWindow(a, nv, tray, onUnlock)
				tray.Attach(nu)
				nu.Show()
			})
			return nil
		})
		tray.Attach(setupW)
		setupW.Show()
		w.Close()
	}
	connectionBtn := widget.NewButtonWithIcon("Connection settings", theme.SettingsIcon(), openSetup)

	// Check first-run status asynchronously to avoid blocking the main thread.
	go func() {
		_, err := v.GetLockedIdentity(context.Background())
		firstRun := errors.Is(err, sharing.ErrNotFound)

		fyne.Do(func() {
			isFirstRun = firstRun

			submit := func(string) {
				if !submitBtn.Disabled() {
					submitBtn.OnTapped()
				}
			}

			var heading string
			var fields fyne.CanvasObject
			if firstRun {
				w.SetTitle("Set Unlock Password")
				heading = "Choose an unlock password for your vault data.\nYou will need this password every time you open Cowbird."
				submitBtn.SetText("Create")
				passwordEntry.OnChanged = func(s string) {
					var score float64
					score, strengthLabel = passwordStrength(s)
					strengthBar.SetValue(score)
				}
				passwordEntry.OnSubmitted = func(string) { w.Canvas().Focus(confirmEntry) }
				confirmEntry.OnSubmitted = submit
				fields = container.NewVBox(passwordEntry, strengthBar, confirmEntry)
			} else {
				w.SetTitle("Unlock Cowbird")
				heading = "Enter your unlock password."
				submitBtn.SetText("Unlock")
				passwordEntry.OnSubmitted = submit
				fields = container.NewVBox(passwordEntry)
			}

			// The recovery/connection actions are pinned to the bottom of the
			// window, side by side; the form sits at the top.
			bottom := container.NewVBox(
				widget.NewSeparator(),
				container.NewGridWithColumns(2, gotoImportBtn, connectionBtn),
			)
			top := container.NewVBox(
				widget.NewLabel(heading),
				widget.NewSeparator(),
				fields,
				submitBtn,
				statusLabel,
			)
			passwordForm = container.NewBorder(nil, bottom, nil, nil, top)
			body.Objects = []fyne.CanvasObject{passwordForm}
			body.Refresh()
			submitBtn.Enable()
			w.Canvas().Focus(passwordEntry)
		})
	}()

	return w
}
