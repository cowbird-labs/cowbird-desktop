package ui

import (
	"fmt"

	"cowbird/internal/auth"
	"cowbird/internal/config"
	"cowbird/internal/credentials"
	"cowbird/internal/vault"

	vaultclient "github.com/hashicorp/vault-client-go"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// SetupDoneFunc is called after successful setup with the saved config,
// selected auth method, and populated credential store.
type SetupDoneFunc func(cfg config.Config, method auth.Method, store credentials.CredentialStore) error

// NewSetupWindow creates the setup window. It is used both for first-run
// configuration and for editing existing connection details; initial pre-fills
// the address, mount path, and auth method (pass a zero config.Config for a
// fresh setup). If creds is non-nil, the credential fields are pre-filled from
// it (so editing the connection does not require re-entering them); pass nil on
// first run to avoid touching the keyring before there is anything to read.
func NewSetupWindow(a fyne.App, initial config.Config, creds credentials.CredentialStore, onComplete SetupDoneFunc) fyne.Window {
	w := a.NewWindow("Cowbird Setup")

	addressEntry := widget.NewEntry()
	addressEntry.SetPlaceHolder("https://vault.example.com:8200")
	addressEntry.SetText(initial.Vault.Address)

	mountEntry := widget.NewEntry()
	if initial.Vault.MountPath != "" {
		mountEntry.SetText(initial.Vault.MountPath)
	} else {
		mountEntry.SetText("cowbird")
	}

	methods := auth.All()
	methodNames := make([]string, len(methods))
	for i, m := range methods {
		methodNames[i] = m.Name()
	}

	statusLabel := widget.NewLabel("")
	connectBtn := widget.NewButton("Connect", nil)

	var selectedMethod auth.Method
	fieldEntries := map[string]*widget.Entry{}
	credContainer := container.NewVBox()

	updateCredFields := func(method auth.Method) {
		selectedMethod = method
		fieldEntries = map[string]*widget.Entry{}
		credContainer.Objects = nil

		for _, f := range method.Fields() {
			var entry *widget.Entry
			if f.Secret {
				entry = widget.NewPasswordEntry()
				// Enter in the secret field (password/token/secret ID)
				// submits the form.
				entry.OnSubmitted = func(string) {
					if !connectBtn.Disabled() {
						connectBtn.OnTapped()
					}
				}
			} else {
				entry = widget.NewEntry()
			}
			// When editing an existing connection, pre-fill from the stored
			// credentials so the user need not retype them. Missing keys (e.g.
			// fields for a method that was never used) simply stay blank.
			if creds != nil {
				if val, err := creds.Get(f.Key); err == nil && val != "" {
					entry.SetText(val)
				}
			}
			fieldEntries[f.Key] = entry
			credContainer.Add(widget.NewLabel(f.Label))
			credContainer.Add(entry)
		}
		credContainer.Refresh()
	}

	// Default to the configured auth method when editing, else the first.
	var defaultMethod auth.Method
	if len(methods) > 0 {
		defaultMethod = methods[0]
		for _, m := range methods {
			if m.Name() == initial.Vault.AuthMethod {
				defaultMethod = m
				break
			}
		}
		updateCredFields(defaultMethod)
	}

	methodSelect := widget.NewSelect(methodNames, func(name string) {
		for _, m := range methods {
			if m.Name() == name {
				updateCredFields(m)
				return
			}
		}
	})
	if defaultMethod != nil {
		methodSelect.SetSelected(defaultMethod.Name())
	}

	setStatus := func(msg string) {
		fyne.Do(func() {
			statusLabel.SetText(msg)
		})
	}

	connectBtn.OnTapped = func() {
		address := addressEntry.Text
		mount := mountEntry.Text
		method := selectedMethod

		values := make(map[string]string, len(fieldEntries))
		for k, e := range fieldEntries {
			values[k] = e.Text
		}

		connectBtn.Disable()
		statusLabel.SetText("Validating...")

		go func() {
			defer fyne.Do(func() { connectBtn.Enable() })

			if address == "" {
				setStatus("Vault address is required")
				return
			}

			if err := method.Validate(values); err != nil {
				setStatus(fmt.Sprintf("Validation error: %v", err))
				return
			}

			setStatus("Saving credentials...")
			store, err := credentials.NewStore("cowbird")
			if err != nil {
				setStatus(fmt.Sprintf("Error creating credential store: %v", err))
				return
			}
			for k, val := range values {
				if err := store.Set(k, val); err != nil {
					setStatus(fmt.Sprintf("Error saving %q: %v", k, err))
					return
				}
			}

			setStatus("Authenticating...")
			client, err := vaultclient.New(vaultclient.WithAddress(address))
			if err != nil {
				setStatus(fmt.Sprintf("Error creating client: %v", err))
				return
			}

			result, err := method.Authenticate(client, store)
			if err != nil {
				setStatus(fmt.Sprintf("Authentication failed: %v", err))
				return
			}
			if err := client.SetToken(result.Token); err != nil {
				setStatus(fmt.Sprintf("Error setting token: %v", err))
				return
			}

			setStatus("Verifying mount path...")
			if err := vault.VerifyMount(client, mount, result.EntityID); err != nil {
				setStatus(fmt.Sprintf("Mount verification failed: %v", err))
				return
			}

			cfg := config.Config{}
			cfg.Vault.Address = address
			cfg.Vault.MountPath = mount
			cfg.Vault.AuthMethod = method.Name()

			setStatus("Saving configuration...")
			if err := config.Save(cfg); err != nil {
				setStatus(fmt.Sprintf("Error saving config: %v", err))
				return
			}

			if err := onComplete(cfg, method, store); err != nil {
				setStatus(fmt.Sprintf("Error: %v", err))
				return
			}

			fyne.Do(func() { w.Close() })
		}()
	}

	serverForm := &widget.Form{
		Items: []*widget.FormItem{
			widget.NewFormItem("Vault Address", addressEntry),
			widget.NewFormItem("Mount Path", mountEntry),
			widget.NewFormItem("Auth Method", methodSelect),
		},
	}

	content := container.NewVBox(
		serverForm,
		widget.NewSeparator(),
		credContainer,
		widget.NewSeparator(),
		connectBtn,
		statusLabel,
	)

	w.SetContent(container.NewPadded(content))
	w.Resize(fyne.NewSize(440, 500))
	w.CenterOnScreen()
	w.Canvas().Focus(addressEntry)

	return w
}
