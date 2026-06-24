package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"

	"cowbird/internal/organization"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// refreshLabelFilter rebuilds the label filter's options from the current
// organization, preserving the selected label across the rebuild. Main thread.
func (m *mainWindow) refreshLabelFilter() {
	prev := m.selectedLabelID()

	options := []string{allLabelsOption}
	selIdx := 0
	if m.org != nil {
		for i, l := range m.org.Labels {
			options = append(options, l.Name)
			if l.ID == prev {
				selIdx = i + 1
			}
		}
	}
	m.labelFilter.Options = options
	m.labelFilter.SetSelectedIndex(selIdx)
	m.labelFilter.Refresh()
}

// selectedLabelID maps the label filter's current selection to a label ID, or ""
// for "All labels". Index-based so duplicate label names stay distinct.
func (m *mainWindow) selectedLabelID() string {
	if m.org == nil || m.labelFilter == nil {
		return ""
	}
	idx := m.labelFilter.SelectedIndex()
	if idx <= 0 || idx-1 >= len(m.org.Labels) {
		return ""
	}
	return m.org.Labels[idx-1].ID
}

// applyOrgChange re-stamps rows from the mutated organization, re-sorts, rebuilds
// the label filter, re-applies filters, and persists the overlay. Main thread.
func (m *mainWindow) applyOrgChange() {
	annotateRows(m.rows, m.org)
	sortRows(m.rows)
	m.refreshLabelFilter()
	m.applyFilter()
	m.persistOrg()
}

// persistOrg saves the organization overlay off the main thread. The record is
// snapshotted via JSON on the main thread first, so a later edit cannot race the
// background marshal. Save failures surface in the status bar.
func (m *mainWindow) persistOrg() {
	data, err := m.org.JSON()
	if err != nil {
		m.setStatus(fmt.Sprintf("Error saving organization: %v", err))
		return
	}
	go func() {
		snapshot, err := organization.ParseOrganization(data)
		if err == nil {
			err = m.app.SaveOrganization(context.Background(), snapshot)
		}
		if err != nil {
			fyne.Do(func() { m.setStatus(fmt.Sprintf("Error saving organization: %v", err)) })
		}
	}()
}

// buildOrgBar renders the per-item favorite toggle and label affordance shown at
// the top of every readable detail pane (owned and shared alike — organization
// is a private overlay that never touches the item itself).
func (m *mainWindow) buildOrgBar(row itemRow) fyne.CanvasObject {
	star := widget.NewButton("", nil)
	star.Importance = widget.LowImportance
	setStar := func(fav bool) {
		if fav {
			star.SetText("★ Favorited")
		} else {
			star.SetText("☆ Favorite")
		}
	}
	setStar(m.org.IsFavorite(row.ID))
	star.OnTapped = func() {
		setStar(m.org.ToggleFavorite(row.ID))
		m.applyOrgChange()
	}

	labelsBtn := widget.NewButtonWithIcon("Labels…", theme.ContentAddIcon(), func() {
		m.showLabelAssignDialog(row)
	})
	labelsBtn.Importance = widget.LowImportance

	top := container.NewBorder(nil, nil, star, labelsBtn, nil)

	if chips := m.labelChips(row.ID); chips != nil {
		return container.NewVBox(top, chips)
	}
	return top
}

// labelChips renders an item's assigned labels as a row of named chips with an
// optional color dot. Returns nil when the item has no labels.
func (m *mainWindow) labelChips(id string) fyne.CanvasObject {
	ids := m.org.LabelsOf(id)
	if len(ids) == 0 {
		return nil
	}
	box := container.NewHBox()
	for _, lid := range ids {
		if l, ok := m.org.Label(lid); ok {
			box.Add(labelChip(l))
		}
	}
	return box
}

// labelChip is a single label's visual: a color dot (when set) plus its name.
func labelChip(l organization.Label) fyne.CanvasObject {
	name := widget.NewLabel(l.Name)
	if c, ok := parseHexColor(l.Color); ok {
		dot := canvas.NewRectangle(c)
		dot.SetMinSize(fyne.NewSize(10, 10))
		dot.CornerRadius = 5
		return container.NewHBox(container.NewCenter(dot), name)
	}
	return container.NewHBox(name)
}

// showLabelAssignDialog lets the user check/uncheck which labels apply to an item
// and optionally create-and-assign a new one in the same step. The dialog's
// content is interactive, so each control forwards Escape (escapableCheck/
// escapableTextEntry) rather than relying on a focused key-catcher — a focused
// catcher would steal focus and block interaction, as it did for Settings.
func (m *mainWindow) showLabelAssignDialog(row itemRow) {
	var d dialog.Dialog
	dismiss := func() { d.Hide() }

	assigned := make(map[string]bool)
	for _, id := range m.org.LabelsOf(row.ID) {
		assigned[id] = true
	}

	checks := make([]*escapableCheck, len(m.org.Labels))
	box := container.NewVBox()
	for i, l := range m.org.Labels {
		c := newEscapableCheck(l.Name, dismiss)
		c.SetChecked(assigned[l.ID])
		checks[i] = c
		box.Add(c)
	}
	if len(m.org.Labels) == 0 {
		box.Add(widget.NewLabel("No labels yet — create one below."))
	}

	newEntry := newEscapableTextEntry(dismiss)
	newEntry.SetPlaceHolder("New label name (optional)")

	content := container.NewVBox(box, widget.NewSeparator(), newEntry)
	title := "Labels"
	if row.Title != "" {
		title = "Labels for " + row.Title
	}
	d = dialog.NewCustomConfirm(title, "Apply", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		for i, l := range m.org.Labels {
			if checks[i].Checked {
				m.org.AssignLabel(row.ID, l.ID)
			} else {
				m.org.UnassignLabel(row.ID, l.ID)
			}
		}
		if name := strings.TrimSpace(newEntry.Text); name != "" {
			if l, err := m.org.AddLabel(name, ""); err == nil {
				m.org.AssignLabel(row.ID, l.ID)
			}
		}
		m.applyOrgChange()
		m.showDetail(row) // re-render so the chips reflect the new assignments
	}, m.win)
	d.Show()
}

// showManageLabelsDialog lets the user rename, recolor, and delete labels. Edits
// apply on Save; deletions strip the label from every item.
func (m *mainWindow) showManageLabelsDialog() {
	type rowEdit struct {
		id      string
		name    *escapableEntry
		color   *escapableEntry
		deleted bool
	}

	var d dialog.Dialog
	dismiss := func() { d.Hide() }

	var edits []*rowEdit
	list := container.NewVBox()
	for _, l := range m.org.Labels {
		re := &rowEdit{id: l.ID}
		re.name = newEscapableTextEntry(dismiss)
		re.name.SetText(l.Name)
		re.color = newEscapableTextEntry(dismiss)
		re.color.SetPlaceHolder("#rrggbb")
		re.color.SetText(l.Color)

		grid := container.NewGridWithColumns(2, re.name, re.color)
		reCapture := re
		var rowContainer *fyne.Container
		delBtn := newEscapableButton("", theme.DeleteIcon(), func() {
			reCapture.deleted = true
			rowContainer.Hide()
		}, dismiss)
		rowContainer = container.NewBorder(nil, nil, nil, delBtn, grid)
		edits = append(edits, re)
		list.Add(rowContainer)
	}
	if len(m.org.Labels) == 0 {
		list.Add(widget.NewLabel("No labels yet."))
	}

	newName := newEscapableTextEntry(dismiss)
	newName.SetPlaceHolder("New label name")
	newColor := newEscapableTextEntry(dismiss)
	newColor.SetPlaceHolder("#rrggbb")

	scroll := container.NewVScroll(list)
	scroll.SetMinSize(fyne.NewSize(380, 240))

	content := container.NewVBox(
		scroll,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Add label", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, newName, newColor),
	)

	d = dialog.NewCustomConfirm("Manage Labels", "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		for _, re := range edits {
			if re.deleted {
				m.org.DeleteLabel(re.id)
				continue
			}
			if name := strings.TrimSpace(re.name.Text); name != "" {
				m.org.RenameLabel(re.id, name)
			}
			m.org.RecolorLabel(re.id, strings.TrimSpace(re.color.Text))
		}
		if name := strings.TrimSpace(newName.Text); name != "" {
			if _, err := m.org.AddLabel(name, strings.TrimSpace(newColor.Text)); err != nil {
				return
			}
		}
		m.applyOrgChange()
	}, m.win)
	d.Resize(fyne.NewSize(440, 420))
	d.Show()
}

// parseHexColor parses a "#rrggbb" string. Returns false for any other form.
func parseHexColor(s string) (color.Color, bool) {
	s = strings.TrimSpace(s)
	if len(s) != 7 || s[0] != '#' {
		return nil, false
	}
	var r, g, b uint8
	n, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b)
	if err != nil || n != 3 {
		return nil, false
	}
	return color.NRGBA{R: r, G: g, B: b, A: 0xff}, true
}
