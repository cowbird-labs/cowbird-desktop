package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const allTypesOption = "All types"
const allLabelsOption = "All labels"

const unreadableTitle = "(unreadable item)"

// favoriteGlyph is the leading marker shown on favorited list rows.
const favoriteGlyph = "★"

// rowStripeColor tints every other list row for readability. A translucent
// mid-gray reads as slightly darker on light themes and slightly lighter on
// dark ones, and its low alpha lets the selection highlight (drawn behind the
// row) still show through on a striped row.
var rowStripeColor = color.NRGBA{R: 128, G: 128, B: 128, A: 0x16}

// buildListPane creates the left pane: search entry, type filter, and the
// item list with an empty-state fallback.
func (m *mainWindow) buildListPane() fyne.CanvasObject {
	// Escape clears the search and resets the type filter to "All types".
	m.search = newEscapableTextEntry(func() {
		m.search.SetText("")
		m.typeFilter.SetSelected(allTypesOption)
	})
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

	m.favFilter = widget.NewCheck("Favorites", func(bool) { m.applyFilter() })

	// Label options are repopulated after each load via refreshLabelFilter; it
	// starts with just the "all" sentinel since no organization is loaded yet.
	m.labelFilter = newEscapableSelect([]string{allLabelsOption}, nil)
	m.labelFilter.SetSelected(allLabelsOption)
	m.labelFilter.OnChanged = func(string) { m.applyFilter() }
	// Escape clears the label filter back to "All labels".
	m.labelFilter.onEscape = func() { m.labelFilter.SetSelected(allLabelsOption) }

	m.list = widget.NewList(
		func() int { return len(m.filtered) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("")
			title.Truncation = fyne.TextTruncateEllipsis
			star := widget.NewLabel("")
			badge := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Italic: true})
			stripe := canvas.NewRectangle(color.Transparent)
			return container.NewStack(stripe, container.NewBorder(nil, nil, star, badge, title))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i >= len(m.filtered) {
				return
			}
			row := m.filtered[i]
			box := o.(*fyne.Container)
			stripe := box.Objects[0].(*canvas.Rectangle)
			content := box.Objects[1].(*fyne.Container)
			// Border children are appended after the center/edge objects, so the
			// star (left) and badge (right) follow the title in Objects.
			title := content.Objects[0].(*widget.Label)
			star := content.Objects[1].(*widget.Label)
			badge := content.Objects[2].(*widget.Label)

			if i%2 == 1 {
				stripe.FillColor = rowStripeColor
			} else {
				stripe.FillColor = color.Transparent
			}
			stripe.Refresh()

			if row.Favorite {
				star.SetText(favoriteGlyph)
			} else {
				star.SetText("")
			}
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

	// Two rows: search + type filter on top, favorites + label filter below, so
	// the controls don't crowd the narrow list pane.
	filtersTop := container.NewBorder(nil, nil, nil, m.typeFilter, m.search)
	filtersBottom := container.NewBorder(nil, nil, m.favFilter, nil, m.labelFilter)
	filters := container.NewVBox(filtersTop, filtersBottom)
	return container.NewBorder(filters, nil, nil, nil, container.NewStack(m.list, m.emptyBox))
}
