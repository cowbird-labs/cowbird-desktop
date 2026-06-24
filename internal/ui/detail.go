package ui

import (
	"errors"
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"time"

	"cowbird/internal/items"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const maskedValue = "••••••••"

// showDetail renders the read-only detail view for row in the right pane.
func (m *mainWindow) showDetail(row itemRow) {
	m.noteActivity()
	if row.Err != nil {
		m.setDetail(m.buildUnreadableDetail(row))
		return
	}

	spec := typeSpecs[row.Type]

	header := widget.NewLabelWithStyle(row.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := spec.display
	if row.Shared {
		subtitle += " · shared by " + m.displayName(row.OwnerID)
	}
	sub := widget.NewLabelWithStyle(subtitle, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	// Field rows are gathered into groups, then rendered as rounded cards with
	// dividers between rows so related fields read as a unit: credentials in one
	// card, websites in their own, and custom fields in another.
	var stdRows, urlRows, customRows []fyne.CanvasObject

	// TOTP rows spin up a background ticker; their stop funcs are collected here
	// and handed to setDetail, which runs them when the pane is next replaced.
	var cleanups []func()

	// Standard fields (skipping the title, which is the header, and empties).
	for _, f := range spec.fields[1:] {
		value := f.get(row.Content)
		if value == "" {
			continue
		}
		if f.totp {
			rowObj, stop := m.buildTOTPRow("One-time code", value)
			stdRows = append(stdRows, rowObj)
			cleanups = append(cleanups, stop)
			continue
		}
		if f.url {
			// One row per URL, sharing a single "Websites" caption on the first.
			for i, u := range splitLines(value) {
				label := ""
				if i == 0 {
					label = "Websites"
				}
				urlRows = append(urlRows, m.buildURLRow(label, u))
			}
			continue
		}
		stdRows = append(stdRows, m.buildFieldRow(f.label, value, f.sensitive, f.grouped))
	}

	// Custom fields.
	for _, cf := range spec.getCustom(row.Content) {
		if cf.Type == items.FieldTOTP {
			rowObj, stop := m.buildTOTPRow(cf.Label, cf.Value)
			customRows = append(customRows, rowObj)
			cleanups = append(cleanups, stop)
			continue
		}
		sensitive := cf.Type == items.FieldHidden
		customRows = append(customRows, m.buildFieldRow(cf.Label, cf.Value, sensitive, false))
	}

	// fields holds everything below the accent header; detailContent pads and
	// pairs it with the header band.
	fields := container.NewVBox()
	// Favorite toggle and labels sit at the top for every readable item, owned
	// or shared — organization is a private overlay, never part of the item.
	if m.org != nil {
		fields.Add(hInset(m.buildOrgBar(row)))
		fields.Add(vSpacer(theme.Padding() * 3))
	}
	if card := fieldCard(stdRows, true); card != nil {
		fields.Add(card)
	}
	if card := fieldCard(urlRows, false); card != nil {
		if len(stdRows) > 0 {
			fields.Add(vSpacer(theme.Padding() * 3))
		}
		fields.Add(card)
	}
	if card := fieldCard(customRows, true); card != nil {
		fields.Add(widget.NewLabelWithStyle("Custom fields", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		fields.Add(card)
	}

	// Shared items are read-only (recipients cannot re-share), so they have no
	// action bar — just the scrolling content.
	if row.Shared {
		m.setDetail(container.NewVScroll(detailContent(row.Type, header, sub, fields)), cleanups...)
		return
	}

	// Owned items can be shared, edited, and deleted. The sharing section is
	// part of the scrolling content; the Edit/Delete actions are pinned to the
	// bottom-right of the pane (separated from the content) so they don't read
	// as part of the sharing section.
	rowCopy := row
	fields.Add(vSpacer(theme.Padding() * 3))
	fields.Add(hInset(m.buildSharingSection(rowCopy)))

	buttons := container.NewHBox(
		widget.NewButtonWithIcon("Edit", theme.DocumentCreateIcon(), func() {
			m.showEditor(rowCopy.Type, &rowCopy)
		}),
		widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			m.confirmDelete(rowCopy)
		}),
	)
	actionBar := container.NewVBox(
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, container.NewPadded(buttons)),
	)

	scroll := container.NewVScroll(detailContent(row.Type, header, sub, fields))
	m.setDetail(container.NewBorder(nil, actionBar, nil, nil, scroll), cleanups...)
}

// vSpacer is a fixed-height transparent gap, used to add deliberate breathing
// room between sections that the default VBox spacing leaves too tight.
func vSpacer(h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(0, h))
	return r
}

// fieldLabel renders a field's caption: a smaller, placeholder-toned line so the
// value beneath it reads as the primary content while staying clearly legible.
// (LowImportance proved too dim against the card background.)
func fieldLabel(text string) fyne.CanvasObject {
	return widget.NewRichText(&widget.TextSegment{
		Text: text,
		Style: widget.RichTextStyle{
			ColorName: theme.ColorNamePlaceHolder,
			SizeName:  theme.SizeNameCaptionText,
			Inline:    true,
		},
	})
}

// buildURLRow renders a single website entry: an optional caption above the URL
// and an "open in browser" button. Websites are meant to be visited, not copied,
// so the action is to open rather than to copy.
func (m *mainWindow) buildURLRow(label, link string) fyne.CanvasObject {
	var name fyne.CanvasObject
	if label != "" {
		name = fieldLabel(label)
	}
	value := widget.NewLabel(link)
	value.Wrapping = fyne.TextWrapWord

	openBtn := ttwidget.NewButtonWithIcon("", theme.MailForwardIcon(), func() {
		m.openURL(link)
	})
	openBtn.SetToolTip("Open in browser")

	return container.NewBorder(name, nil, nil, openBtn, value)
}

// openURL opens link in the user's default browser. A link with no scheme is
// assumed to be https, so entries stored as "example.com" still open. Failures
// surface in the status bar rather than as a dialog.
func (m *mainWindow) openURL(link string) {
	m.noteActivity()
	link = strings.TrimSpace(link)
	if link == "" {
		return
	}
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	u, err := url.Parse(link)
	if err != nil {
		m.setStatus(fmt.Sprintf("Invalid URL: %v", err))
		return
	}
	if err := fyne.CurrentApp().OpenURL(u); err != nil {
		m.setStatus(fmt.Sprintf("Could not open URL: %v", err))
	}
}

// fieldCard groups related field rows into a single rounded panel so they read
// as a unit rather than floating in a flat list. With dividers, thin separators
// are drawn between rows; without, the rows sit flush (used for the websites
// list). Returns nil for an empty group so callers can skip it.
func fieldCard(rows []fyne.CanvasObject, dividers bool) fyne.CanvasObject {
	if len(rows) == 0 {
		return nil
	}
	inner := container.NewVBox()
	for i, r := range rows {
		if i > 0 && dividers {
			inner.Add(widget.NewSeparator())
		}
		inner.Add(r)
	}
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.CornerRadius = theme.Padding() * 2
	card := container.NewStack(bg, container.NewPadded(inner))
	return hInset(card)
}

// sectionHInset is the fraction of the pane width used as left/right spacing on
// each side of a detail-view section (cards and the sharing block), so they all
// align to the same proportional margin.
const sectionHInset = 0.025

// hInset wraps obj with the standard proportional left/right section spacing.
func hInset(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.New(hPadLayout{frac: sectionHInset}, obj)
}

// hPadLayout insets its single child horizontally by frac of the container width
// on each side, giving sections proportional left/right spacing that scales with
// the pane. Vertical sizing/position is left to the child.
type hPadLayout struct{ frac float32 }

func (l hPadLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) == 0 {
		return
	}
	pad := size.Width * l.frac
	objs[0].Move(fyne.NewPos(pad, 0))
	objs[0].Resize(fyne.NewSize(size.Width-2*pad, size.Height))
}

func (l hPadLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	return objs[0].MinSize()
}

// detailContent assembles the accent header and the field list into the
// scrollable detail body. The band runs edge-to-edge across the top of the pane
// (escaping the outer padding); only the field list below it is padded. The
// band is tinted per item type so types are distinguishable at a glance, with a
// translucent accent that keeps the themed title text legible.
func detailContent(t items.ItemType, header, sub, fields fyne.CanvasObject) fyne.CanvasObject {
	title := container.NewPadded(container.NewVBox(header, sub))
	band := canvas.NewRectangle(typeColor(t))
	return container.NewVBox(
		container.NewStack(band, title),
		container.NewPadded(fields),
	)
}

// buildFieldRow renders one labeled value with a copy button and, for
// sensitive values, a reveal toggle. Copy always yields the raw value; grouped
// only changes how the value is displayed (e.g. a card number shown in groups
// of four), never what is copied.
func (m *mainWindow) buildFieldRow(label, value string, sensitive, grouped bool) fyne.CanvasObject {
	// An empty label yields a caption-less row, used for the 2nd+ entries in a
	// multi-value group (e.g. additional websites) that share one caption above.
	var name fyne.CanvasObject
	if label != "" {
		name = fieldLabel(label)
	}

	// display is what the user sees when the value is revealed; copy always uses
	// the raw value so formatting never leaks into the clipboard.
	display := value
	var valueLabel *widget.Label
	if grouped {
		display = groupDigits(strings.ReplaceAll(value, " ", ""), 4)
		valueLabel = widget.NewLabelWithStyle(display, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	} else {
		valueLabel = widget.NewLabel(display)
	}
	valueLabel.Wrapping = fyne.TextWrapWord

	actions := container.NewHBox()
	if sensitive {
		valueLabel.SetText(maskedValue)
		revealed := false
		var revealBtn *ttwidget.Button
		revealBtn = ttwidget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
			revealed = !revealed
			if revealed {
				valueLabel.SetText(display)
				revealBtn.SetIcon(theme.VisibilityOffIcon())
				revealBtn.SetToolTip("Hide")
			} else {
				valueLabel.SetText(maskedValue)
				revealBtn.SetIcon(theme.VisibilityIcon())
				revealBtn.SetToolTip("Reveal")
			}
		})
		revealBtn.SetToolTip("Reveal")
		actions.Add(revealBtn)
	}
	copyBtn := ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		m.copyToClipboard(value, fmt.Sprintf("Copied %s", label))
	})
	copyBtn.SetToolTip("Copy to clipboard")
	actions.Add(copyBtn)

	return container.NewBorder(name, nil, nil, actions, valueLabel)
}

// buildTOTPRow renders a live one-time code derived from a stored TOTP secret,
// refreshed every second with a countdown to the next rotation. Copy yields the
// current digits (no spacing). The returned stop func ends the refresh ticker
// and must be called when the detail pane is replaced; setDetail handles that.
func (m *mainWindow) buildTOTPRow(label, secret string) (fyne.CanvasObject, func()) {
	name := fieldLabel(label)
	valueLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})

	// If the secret is unusable there is nothing to refresh; render a static
	// message and hand back a no-op stop.
	if _, _, err := totpNow(secret); err != nil {
		valueLabel.SetText("Invalid TOTP secret")
		return container.NewBorder(name, nil, nil, nil, valueLabel), func() {}
	}

	countdown := widget.NewLabel("")

	// current holds the raw digits for copying. It is only ever read/written on
	// the main thread (the initial call below and via fyne.Do in the ticker).
	var current string
	update := func() {
		code, remaining, err := totpNow(secret)
		if err != nil {
			valueLabel.SetText("Invalid TOTP secret")
			countdown.SetText("")
			return
		}
		current = code
		valueLabel.SetText(groupDigits(code, 3))
		countdown.SetText(fmt.Sprintf("%2ds", remaining))
	}
	update()

	copyBtn := ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		m.copyToClipboard(current, fmt.Sprintf("Copied %s", label))
	})
	copyBtn.SetToolTip("Copy code")

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				fyne.Do(update)
			}
		}
	}()

	actions := container.NewHBox(countdown, copyBtn)
	return container.NewBorder(name, nil, nil, actions, valueLabel), func() { close(stop) }
}

// buildUnreadableDetail explains a row whose content failed to decrypt or
// decode, including the likely cause and what the user can do. The item may
// still be deleted if owned.
func (m *mainWindow) buildUnreadableDetail(row itemRow) fyne.CanvasObject {
	msg := widget.NewLabel(fmt.Sprintf("This item could not be read:\n%v", row.Err))
	msg.Wrapping = fyne.TextWrapWord

	// The most common cause differs by ownership. A shared item typically goes
	// unreadable when its owner rotated their key and has not re-shared it yet
	// (see CLAUDE.md: rotation does not re-key items shared *with* you). An owned
	// item is more likely corrupted storage or a key mismatch.
	var guidance string
	if row.Shared {
		guidance = "This is usually because " + m.displayName(row.OwnerID) +
			" rotated their key and has not re-shared this item yet. " +
			"Ask them to share it again. It will stay unreadable until they do."
	} else {
		guidance = "The stored data may be corrupted or was encrypted under a " +
			"different key. If you cannot recover it, you can delete it below."
	}
	help := widget.NewLabel(guidance)
	help.Wrapping = fyne.TextWrapWord

	body := container.NewVBox(
		widget.NewLabelWithStyle(unreadableTitle, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		msg,
		help,
	)
	if !row.Shared {
		rowCopy := row
		body.Add(widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			m.confirmDelete(rowCopy)
		}))
	}
	return container.NewVScroll(container.NewPadded(body))
}

// groupDigits inserts a space every size characters, e.g. a 16-digit card
// number becomes "4242 4242 4242 4242". A trailing short group is kept as-is.
func groupDigits(s string, size int) string {
	if size <= 0 || len(s) <= size {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%size == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// totpNow generates the current code for a stored TOTP value. The value may be a
// bare base32 secret or a full otpauth:// URI (as exported by other password
// managers) — the latter is parsed for its secret and parameters (period,
// digits, algorithm), so non-default configurations render correctly. Internal
// spaces (common in grouped secrets) are stripped; the otp library handles
// padding and case. It returns the code, the seconds remaining in the period,
// and an error for an empty or malformed value.
func totpNow(value string) (code string, remaining int, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0, errors.New("empty TOTP secret")
	}
	now := time.Now()

	// otpauth:// URI (the form other managers export): parse its secret and
	// parameters so non-default period/digits/algorithm render correctly.
	if strings.HasPrefix(strings.ToLower(value), "otpauth://") {
		key, err := otp.NewKeyFromURL(value)
		if err != nil {
			return "", 0, err
		}
		period := int(key.Period())
		if period <= 0 {
			period = 30
		}
		code, err = totp.GenerateCodeCustom(key.Secret(), now, totp.ValidateOpts{
			Period:    uint(period),
			Digits:    key.Digits(),
			Algorithm: key.Algorithm(),
		})
		if err != nil {
			return "", 0, err
		}
		return code, period - int(now.Unix()%int64(period)), nil
	}

	// Bare base32 secret (default SHA1 / 6 digits / 30s, as GenerateCode sets).
	secret := strings.ReplaceAll(value, " ", "")
	code, err = totp.GenerateCode(secret, now)
	if err != nil {
		return "", 0, err
	}
	const period = 30
	return code, period - int(now.Unix()%period), nil
}
