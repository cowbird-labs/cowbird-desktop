package ui

import (
	"fmt"
	"log"

	"cowbird/internal/config"
	"cowbird/internal/generate"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	modePassword   = "Password"
	modePassphrase = "Passphrase"

	genLengthMin = 8
	genLengthMax = 64
	genWordsMin  = 3
	genWordsMax  = 12
)

// showGeneratorDialog opens the password/passphrase generator. When onUse is
// non-nil (inline use from an editor field), the dialog offers a "Use" action
// that passes the current value to onUse; when nil (standalone, from the main
// menu) it offers a "Copy" button instead. Last-used settings are read from and
// written back to the config file.
func (m *mainWindow) showGeneratorDialog(onUse func(string)) {
	// Load the full config so persisting the generator settings does not clobber
	// other sections (Save merges the whole struct back).
	cfg, err := config.Load()
	if err != nil {
		// Fall back to defaults in memory; persistence will just be skipped.
		cfg = config.Config{Generator: config.Generator{
			Mode: "password", Length: 20, Lower: true, Upper: true,
			Digits: true, Symbols: true, Words: 5, Separator: "-",
			Capitalize: true, IncludeNumber: true,
		}}
		log.Printf("generator: loading config: %v", err)
	}
	g := cfg.Generator

	// d is created below; dismiss closes the dialog (without firing the "Use"
	// callback) and is wired to Escape on every focusable control, since Fyne
	// routes key events only to the focused widget and dialogs do not dismiss on
	// Escape themselves.
	var d dialog.Dialog
	dismiss := func() { d.Hide() }

	// current holds the latest generated value for Use/Copy.
	var current string

	valueLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	valueLabel.Wrapping = fyne.TextWrapBreak

	strengthLabel := ""
	strengthBar := widget.NewProgressBar()
	strengthBar.TextFormatter = func() string { return strengthLabel }

	// --- Password controls ---
	lengthVal := widget.NewLabel("")
	lengthSlider := newEscapableSlider(genLengthMin, genLengthMax, dismiss)
	lowerCheck := newEscapableCheck("a-z", dismiss)
	upperCheck := newEscapableCheck("A-Z", dismiss)
	digitsCheck := newEscapableCheck("0-9", dismiss)
	symbolsCheck := newEscapableCheck("!@#", dismiss)
	ambiguousCheck := newEscapableCheck("Exclude ambiguous (il1Io0|)", dismiss)

	// --- Passphrase controls ---
	wordsVal := widget.NewLabel("")
	wordsSlider := newEscapableSlider(genWordsMin, genWordsMax, dismiss)
	separatorEntry := newEscapableTextEntry(dismiss)
	capitalizeCheck := newEscapableCheck("Capitalize words", dismiss)
	numberCheck := newEscapableCheck("Include a number", dismiss)

	passwordOptsFromUI := func() generate.PasswordOpts {
		return generate.PasswordOpts{
			Length:           int(lengthSlider.Value),
			Lower:            lowerCheck.Checked,
			Upper:            upperCheck.Checked,
			Digits:           digitsCheck.Checked,
			Symbols:          symbolsCheck.Checked,
			ExcludeAmbiguous: ambiguousCheck.Checked,
		}
	}
	passphraseOptsFromUI := func() generate.PassphraseOpts {
		return generate.PassphraseOpts{
			Words:         int(wordsSlider.Value),
			Separator:     separatorEntry.Text,
			Capitalize:    capitalizeCheck.Checked,
			IncludeNumber: numberCheck.Checked,
		}
	}

	modeRadio := newEscapableRadioGroup([]string{modePassword, modePassphrase}, nil)
	modeRadio.Horizontal = true
	modeRadio.onEscape = dismiss

	passwordBox := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Length"), lengthVal, lengthSlider),
		container.NewGridWithColumns(4, lowerCheck, upperCheck, digitsCheck, symbolsCheck),
		ambiguousCheck,
	)
	passphraseBox := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Words"), wordsVal, wordsSlider),
		container.NewBorder(nil, nil, widget.NewLabel("Separator"), nil, separatorEntry),
		capitalizeCheck,
		numberCheck,
	)
	controls := container.NewStack(passwordBox, passphraseBox)

	// persist writes the current UI state back to the config file (best effort).
	persist := func() {
		cfg.Generator = config.Generator{
			Mode:             "password",
			Length:           int(lengthSlider.Value),
			Lower:            lowerCheck.Checked,
			Upper:            upperCheck.Checked,
			Digits:           digitsCheck.Checked,
			Symbols:          symbolsCheck.Checked,
			ExcludeAmbiguous: ambiguousCheck.Checked,
			Words:            int(wordsSlider.Value),
			Separator:        separatorEntry.Text,
			Capitalize:       capitalizeCheck.Checked,
			IncludeNumber:    numberCheck.Checked,
		}
		if modeRadio.Selected == modePassphrase {
			cfg.Generator.Mode = "passphrase"
		}
		if err := config.Save(cfg); err != nil {
			log.Printf("generator: saving settings: %v", err)
		}
	}

	regenerate := func() {
		lengthVal.SetText(fmt.Sprintf("%d", int(lengthSlider.Value)))
		wordsVal.SetText(fmt.Sprintf("%d", int(wordsSlider.Value)))

		var value string
		var score float64
		var genErr error
		if modeRadio.Selected == modePassphrase {
			opts := passphraseOptsFromUI()
			value, genErr = generate.Passphrase(opts)
			score, strengthLabel = strengthFromBits(opts.Entropy())
		} else {
			opts := passwordOptsFromUI()
			value, genErr = generate.Password(opts)
			score, strengthLabel = passwordStrength(value)
		}
		if genErr != nil {
			current = ""
			valueLabel.SetText(genErr.Error())
			strengthBar.SetValue(0)
			return
		}
		current = value
		valueLabel.SetText(value)
		strengthBar.SetValue(score)
		persist()
	}

	// guardClasses keeps at least one password character class enabled: toggling
	// off the last one re-checks it instead of producing an impossible request.
	guardClasses := func(c *escapableCheck) func(bool) {
		return func(bool) {
			if !lowerCheck.Checked && !upperCheck.Checked && !digitsCheck.Checked && !symbolsCheck.Checked {
				c.SetChecked(true) // re-check; SetChecked re-invokes OnChanged but the guard now passes
				return
			}
			regenerate()
		}
	}
	lowerCheck.OnChanged = guardClasses(lowerCheck)
	upperCheck.OnChanged = guardClasses(upperCheck)
	digitsCheck.OnChanged = guardClasses(digitsCheck)
	symbolsCheck.OnChanged = guardClasses(symbolsCheck)
	ambiguousCheck.OnChanged = func(bool) { regenerate() }
	capitalizeCheck.OnChanged = func(bool) { regenerate() }
	numberCheck.OnChanged = func(bool) { regenerate() }
	lengthSlider.OnChanged = func(float64) { regenerate() }
	wordsSlider.OnChanged = func(float64) { regenerate() }
	separatorEntry.OnChanged = func(string) { regenerate() }

	showMode := func(mode string) {
		if mode == modePassphrase {
			passwordBox.Hide()
			passphraseBox.Show()
		} else {
			passphraseBox.Hide()
			passwordBox.Show()
		}
	}
	modeRadio.OnChanged = func(mode string) {
		showMode(mode)
		regenerate()
	}

	// Seed the widgets from saved settings (without firing regenerate per set).
	lengthSlider.Value = clampFloat(float64(g.Length), genLengthMin, genLengthMax)
	wordsSlider.Value = clampFloat(float64(g.Words), genWordsMin, genWordsMax)
	lowerCheck.Checked = g.Lower
	upperCheck.Checked = g.Upper
	digitsCheck.Checked = g.Digits
	symbolsCheck.Checked = g.Symbols
	ambiguousCheck.Checked = g.ExcludeAmbiguous
	separatorEntry.SetText(g.Separator)
	capitalizeCheck.Checked = g.Capitalize
	numberCheck.Checked = g.IncludeNumber
	// Guarantee at least one class is on, even if a hand-edited config disabled all.
	if !lowerCheck.Checked && !upperCheck.Checked && !digitsCheck.Checked && !symbolsCheck.Checked {
		lowerCheck.Checked = true
	}

	regenBtn := newEscapableButton("Regenerate", theme.ViewRefreshIcon(), regenerate, dismiss)
	valueRow := container.NewBorder(nil, nil, nil, regenBtn, valueLabel)

	body := container.NewVBox(
		modeRadio,
		widget.NewSeparator(),
		controls,
		widget.NewSeparator(),
		valueRow,
		strengthBar,
	)

	if onUse != nil {
		d = dialog.NewCustomConfirm("Generate", "Use", "Cancel", body, func(use bool) {
			if use && current != "" {
				onUse(current)
			}
		}, m.win)
	} else {
		copyBtn := newEscapableButton("Copy", theme.ContentCopyIcon(), func() {
			if current != "" {
				m.win.Clipboard().SetContent(current)
				m.status.SetText("Copied generated value")
			}
		}, dismiss)
		body.Add(container.NewHBox(copyBtn))
		d = dialog.NewCustom("Generate", "Close", body, m.win)
	}
	d.Resize(fyne.NewSize(440, 420))

	// Escape fallback for the brief window after Show when no control yet holds
	// focus (focused controls forward Escape themselves via dismiss). Restore
	// any prior handler when the dialog closes.
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

	// Initial state: select the saved mode (fires OnChanged → showMode + first
	// generate) and reveal the dialog.
	if g.Mode == "passphrase" {
		modeRadio.SetSelected(modePassphrase)
	} else {
		modeRadio.SetSelected(modePassword)
	}
	d.Show()
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// The escapable* widgets below forward the Escape key to onEscape so the
// generator dialog can be dismissed no matter which control holds focus —
// Fyne's base Check/Slider/Button swallow Escape in their TypedKey. The text
// entry and radio variants already exist (password.go, app.go).

type escapableCheck struct {
	widget.Check
	onEscape func()
}

func newEscapableCheck(label string, onEscape func()) *escapableCheck {
	c := &escapableCheck{onEscape: onEscape}
	c.Text = label
	c.ExtendBaseWidget(c)
	return c
}

func (c *escapableCheck) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && c.onEscape != nil {
		c.onEscape()
		return
	}
	c.Check.TypedKey(key)
}

type escapableSlider struct {
	widget.Slider
	onEscape func()
}

func newEscapableSlider(lo, hi float64, onEscape func()) *escapableSlider {
	s := &escapableSlider{onEscape: onEscape}
	s.Min = lo
	s.Max = hi
	s.Step = 1
	s.ExtendBaseWidget(s)
	return s
}

func (s *escapableSlider) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && s.onEscape != nil {
		s.onEscape()
		return
	}
	s.Slider.TypedKey(key)
}

type escapableButton struct {
	widget.Button
	onEscape func()
}

func newEscapableButton(label string, icon fyne.Resource, tapped, onEscape func()) *escapableButton {
	b := &escapableButton{onEscape: onEscape}
	b.Text = label
	b.Icon = icon
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

func (b *escapableButton) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && b.onEscape != nil {
		b.onEscape()
		return
	}
	b.Button.TypedKey(key)
}
