package ui

import (
	"fmt"

	"cowbird/internal/items"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const maskedValue = "••••••••"

// showDetail renders the read-only detail view for row in the right pane.
func (m *mainWindow) showDetail(row itemRow) {
	if row.Err != nil {
		m.setDetail(m.buildUnreadableDetail(row))
		return
	}

	spec := typeSpecs[row.Type]

	header := widget.NewLabelWithStyle(row.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := spec.display
	if row.Shared {
		subtitle += " · shared by " + row.OwnerID
	}
	sub := widget.NewLabelWithStyle(subtitle, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	body := container.NewVBox(header, sub, widget.NewSeparator())

	// Standard fields (skipping the title, which is the header, and empties).
	for _, f := range spec.fields[1:] {
		value := f.get(row.Content)
		if value == "" {
			continue
		}
		body.Add(m.buildFieldRow(f.label, value, f.sensitive))
	}

	// Custom fields.
	for _, cf := range spec.getCustom(row.Content) {
		sensitive := cf.Type == items.FieldHidden || cf.Type == items.FieldTOTP
		body.Add(m.buildFieldRow(cf.Label, cf.Value, sensitive))
	}

	// Owned items can be edited and deleted; shared items are read-only.
	if !row.Shared {
		rowCopy := row
		buttons := container.NewHBox(
			widget.NewButtonWithIcon("Edit", theme.DocumentCreateIcon(), func() {
				m.showEditor(rowCopy.Type, &rowCopy)
			}),
			widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
				m.confirmDelete(rowCopy)
			}),
		)
		body.Add(widget.NewSeparator())
		body.Add(buttons)
	}

	m.setDetail(container.NewVScroll(container.NewPadded(body)))
}

// buildFieldRow renders one labeled value with a copy button and, for
// sensitive values, a reveal toggle. Copy never requires revealing.
func (m *mainWindow) buildFieldRow(label, value string, sensitive bool) fyne.CanvasObject {
	name := widget.NewLabelWithStyle(label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	valueLabel := widget.NewLabel(value)
	valueLabel.Wrapping = fyne.TextWrapWord

	actions := container.NewHBox()
	if sensitive {
		valueLabel.SetText(maskedValue)
		revealed := false
		var revealBtn *widget.Button
		revealBtn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
			revealed = !revealed
			if revealed {
				valueLabel.SetText(value)
				revealBtn.SetIcon(theme.VisibilityOffIcon())
			} else {
				valueLabel.SetText(maskedValue)
				revealBtn.SetIcon(theme.VisibilityIcon())
			}
		})
		actions.Add(revealBtn)
	}
	actions.Add(widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		m.win.Clipboard().SetContent(value)
		m.status.SetText(fmt.Sprintf("Copied %s", label))
	}))

	return container.NewBorder(name, nil, nil, actions, valueLabel)
}

// buildUnreadableDetail explains a row whose content failed to decrypt or
// decode. The item may still be deleted if owned.
func (m *mainWindow) buildUnreadableDetail(row itemRow) fyne.CanvasObject {
	msg := widget.NewLabel(fmt.Sprintf("This item could not be read:\n%v", row.Err))
	msg.Wrapping = fyne.TextWrapWord

	body := container.NewVBox(
		widget.NewLabelWithStyle(unreadableTitle, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		msg,
	)
	if !row.Shared {
		rowCopy := row
		body.Add(widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			m.confirmDelete(rowCopy)
		}))
	}
	return container.NewVScroll(container.NewPadded(body))
}
