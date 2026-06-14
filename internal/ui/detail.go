package ui

import (
	"fmt"

	"cowbird/internal/items"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
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
		subtitle += " · shared by " + m.displayName(row.OwnerID)
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

	// Shared items are read-only (recipients cannot re-share), so they have no
	// action bar — just the scrolling content.
	if row.Shared {
		m.setDetail(container.NewVScroll(container.NewPadded(body)))
		return
	}

	// Owned items can be shared, edited, and deleted. The sharing section is
	// part of the scrolling content; the Edit/Delete actions are pinned to the
	// bottom-right of the pane (separated from the content) so they don't read
	// as part of the sharing section.
	rowCopy := row
	body.Add(m.buildSharingSection(rowCopy))

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

	scroll := container.NewVScroll(container.NewPadded(body))
	m.setDetail(container.NewBorder(nil, actionBar, nil, nil, scroll))
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
		var revealBtn *ttwidget.Button
		revealBtn = ttwidget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
			revealed = !revealed
			if revealed {
				valueLabel.SetText(value)
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
		m.win.Clipboard().SetContent(value)
		m.status.SetText(fmt.Sprintf("Copied %s", label))
	})
	copyBtn.SetToolTip("Copy to clipboard")
	actions.Add(copyBtn)

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
