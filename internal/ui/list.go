package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const allTypesOption = "All types"

const unreadableTitle = "(unreadable item)"

// buildListPane creates the left pane: search entry, type filter, and the
// item list with an empty-state fallback.
func (m *mainWindow) buildListPane() fyne.CanvasObject {
	m.search = widget.NewEntry()
	m.search.SetPlaceHolder("Search…")
	m.search.OnChanged = func(string) { m.applyFilter() }

	options := []string{allTypesOption}
	for _, t := range typeOrder {
		options = append(options, typeSpecs[t].display)
	}
	// The OnChanged callback is attached after the initial SetSelected so it
	// does not fire applyFilter before the list widget below exists.
	m.typeFilter = widget.NewSelect(options, nil)
	m.typeFilter.SetSelected(allTypesOption)
	m.typeFilter.OnChanged = func(string) { m.applyFilter() }

	m.list = widget.NewList(
		func() int { return len(m.filtered) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("")
			title.Truncation = fyne.TextTruncateEllipsis
			badge := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Italic: true})
			return container.NewBorder(nil, nil, nil, badge, title)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i >= len(m.filtered) {
				return
			}
			row := m.filtered[i]
			box := o.(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			badge := box.Objects[1].(*widget.Label)

			if row.Err != nil {
				title.SetText(unreadableTitle)
			} else {
				title.SetText(row.Title)
			}
			badgeText := typeSpecs[row.Type].display
			if row.Shared {
				badgeText += " · shared"
			}
			badge.SetText(badgeText)
		},
	)
	m.list.OnSelected = func(i widget.ListItemID) {
		if i < len(m.filtered) {
			m.showDetail(m.filtered[i])
		}
	}

	emptyLabel := widget.NewLabel("No items yet.\nUse + to create your first item.")
	emptyLabel.Alignment = fyne.TextAlignCenter
	m.emptyBox = container.NewCenter(emptyLabel)
	m.emptyBox.Hide()

	filters := container.NewBorder(nil, nil, nil, m.typeFilter, m.search)
	return container.NewBorder(filters, nil, nil, nil, container.NewMax(m.list, m.emptyBox))
}
