package ui

import (
	"context"
	"fmt"
	"strings"

	"cowbird/internal/items"
	"cowbird/internal/sharing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// customFieldRow is one editable custom field in the editor.
type customFieldRow struct {
	kind  *widget.Select
	label *widget.Entry
	value *widget.Entry
	box   fyne.CanvasObject
}

// showEditor renders the editor in the right pane. row nil means create a new
// item of type typ; otherwise edit row's content in place.
func (m *mainWindow) showEditor(typ items.ItemType, row *itemRow) {
	spec := typeSpecs[typ]

	content := spec.empty()
	heading := "New " + spec.display
	if row != nil {
		content = row.Content
		heading = "Edit " + spec.display
	}

	errLabel := widget.NewLabel("")
	errLabel.Importance = widget.DangerImportance
	errLabel.Hide()

	// Standard field entries, in spec order.
	entries := make([]*widget.Entry, len(spec.fields))
	form := widget.NewForm()
	for i, f := range spec.fields {
		var entry *widget.Entry
		switch {
		case f.sensitive:
			entry = widget.NewPasswordEntry()
		case f.multiline:
			entry = widget.NewMultiLineEntry()
			entry.Wrapping = fyne.TextWrapWord
			entry.SetMinRowsVisible(3)
		default:
			entry = widget.NewEntry()
		}
		entry.SetText(f.get(content))
		entries[i] = entry
		form.Append(f.label, entry)
	}

	// Custom fields: a repeater with add/remove rows.
	var customRows []*customFieldRow
	customBox := container.NewVBox()

	addCustomRow := func(cf items.Field) {
		kindOptions := make([]string, len(customFieldKinds))
		for i, k := range customFieldKinds {
			kindOptions[i] = k.display
		}
		r := &customFieldRow{
			kind:  widget.NewSelect(kindOptions, nil),
			label: widget.NewEntry(),
			value: widget.NewEntry(),
		}
		r.kind.SetSelected(kindDisplay(cf.Type))
		r.label.SetPlaceHolder("Label")
		r.label.SetText(cf.Label)
		r.value.SetPlaceHolder("Value")
		r.value.SetText(cf.Value)

		var removeBtn *widget.Button
		removeBtn = widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)
		fields := container.NewGridWithColumns(3, r.kind, r.label, r.value)
		r.box = container.NewBorder(nil, nil, nil, removeBtn, fields)
		removeBtn.OnTapped = func() {
			for i, cr := range customRows {
				if cr == r {
					customRows = append(customRows[:i], customRows[i+1:]...)
					break
				}
			}
			customBox.Remove(r.box)
		}

		customRows = append(customRows, r)
		customBox.Add(r.box)
	}

	for _, cf := range spec.getCustom(content) {
		addCustomRow(cf)
	}
	addFieldBtn := widget.NewButtonWithIcon("Add custom field", theme.ContentAddIcon(), func() {
		addCustomRow(items.Field{Type: items.FieldText})
	})

	cancel := func() {
		if row != nil {
			m.showDetail(*row)
		} else {
			m.setDetail(detailPlaceholder())
		}
	}

	var saveBtn *widget.Button
	saveBtn = widget.NewButtonWithIcon("Save", theme.ConfirmIcon(), func() {
		showErr := func(msg string) {
			errLabel.SetText(msg)
			errLabel.Show()
		}

		// Validate, then build the content value from the form.
		updated := content
		for i, f := range spec.fields {
			text := entries[i].Text
			if f.required && strings.TrimSpace(text) == "" {
				showErr(f.label + " is required")
				return
			}
			updated = f.set(updated, text)
		}

		var customFields []items.Field
		for _, r := range customRows {
			label := strings.TrimSpace(r.label.Text)
			if label == "" && r.value.Text == "" {
				continue // fully empty row — drop silently
			}
			if label == "" {
				showErr("Custom fields need a label")
				return
			}
			customFields = append(customFields, items.Field{
				Type:  kindFromDisplay(r.kind.Selected),
				Label: label,
				Value: r.value.Text,
			})
		}
		updated = spec.setCustom(updated, customFields)

		errLabel.Hide()
		saveBtn.Disable()
		m.status.SetText("Saving…")

		go func() {
			var id string
			var err error
			if row != nil {
				_, err = m.app.Service.UpdateItem(context.Background(), row.ID, updated)
				id = row.ID
			} else {
				var env sharing.Envelope
				env, err = m.app.Service.CreateItem(context.Background(), updated)
				id = env.ID
			}
			fyne.Do(func() {
				saveBtn.Enable()
				if err != nil {
					m.status.SetText(fmt.Sprintf("Error saving item: %v", err))
					return
				}
				m.status.SetText("")
				m.showDetail(itemRow{ID: id, Title: titleOf(updated), Type: updated.Kind(), Content: updated})
				m.reload()
			})
		}()
	})
	cancelBtn := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), cancel)

	body := container.NewVBox(
		widget.NewLabelWithStyle(heading, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		form,
		widget.NewLabelWithStyle("Custom fields", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		customBox,
		addFieldBtn,
		errLabel,
		container.NewHBox(saveBtn, cancelBtn),
	)
	m.setDetail(container.NewVScroll(container.NewPadded(body)))
}
