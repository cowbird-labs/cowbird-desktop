package ui

import (
	"context"
	"fmt"

	"cowbird/internal/core"
	"cowbird/internal/items"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// mainWindow holds the state of the item list / editor window.
type mainWindow struct {
	app *core.App
	win fyne.Window

	rows     []itemRow // everything loaded, sorted by title
	filtered []itemRow // rows matching the current search/type filter

	search     *widget.Entry
	typeFilter *widget.Select
	list       *widget.List
	emptyBox   *fyne.Container // shown instead of the list when there are no items
	detail     *fyne.Container // right pane, single slot
	status     *widget.Label
	retryBtn   *widget.Button
}

// NewMainWindow creates the main item list / editor window.
func NewMainWindow(a fyne.App, app *core.App) fyne.Window {
	m := &mainWindow{app: app, win: a.NewWindow("Cowbird")}
	m.win.Resize(fyne.NewSize(900, 600))
	m.win.CenterOnScreen()

	m.status = widget.NewLabel("")
	m.retryBtn = widget.NewButton("Retry", func() { m.reload() })
	m.retryBtn.Hide()

	m.detail = container.NewMax(detailPlaceholder())

	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.ContentAddIcon(), m.chooseNewItemType),
		widget.NewToolbarAction(theme.ViewRefreshIcon(), m.reload),
		widget.NewToolbarSpacer(),
	)

	split := container.NewHSplit(m.buildListPane(), m.detail)
	split.SetOffset(0.35)

	statusBar := container.NewBorder(nil, nil, nil, m.retryBtn, m.status)
	m.win.SetContent(container.NewBorder(toolbar, statusBar, nil, nil, split))

	m.reload()
	return m.win
}

// reload processes the inbox and reloads all items off the main thread, then
// updates the list. Errors land in the status bar with a Retry action.
func (m *mainWindow) reload() {
	m.status.SetText("Loading…")
	m.retryBtn.Hide()

	go func() {
		rows, err := loadRows(context.Background(), m.app)
		fyne.Do(func() {
			if err != nil {
				m.status.SetText(fmt.Sprintf("Error loading items: %v", err))
				m.retryBtn.Show()
				return
			}
			m.status.SetText("")
			m.rows = rows
			m.applyFilter()
		})
	}()
}

// applyFilter rebuilds the filtered rows from the current search string and
// type filter and refreshes the list. Main thread only.
func (m *mainWindow) applyFilter() {
	search := m.search.Text
	typ := items.ItemType("")
	if sel := m.typeFilter.Selected; sel != "" && sel != allTypesOption {
		for _, t := range typeOrder {
			if typeSpecs[t].display == sel {
				typ = t
				break
			}
		}
	}

	m.filtered = m.filtered[:0]
	for _, row := range m.rows {
		if row.matchesFilter(search, typ) {
			m.filtered = append(m.filtered, row)
		}
	}

	m.list.UnselectAll()
	m.list.Refresh()
	if len(m.rows) == 0 {
		m.emptyBox.Show()
		m.list.Hide()
	} else {
		m.emptyBox.Hide()
		m.list.Show()
	}
}

// setDetail swaps the right pane's content. Main thread only.
func (m *mainWindow) setDetail(o fyne.CanvasObject) {
	m.detail.Objects = []fyne.CanvasObject{o}
	m.detail.Refresh()
}

func detailPlaceholder() fyne.CanvasObject {
	return container.NewCenter(widget.NewLabel("Select an item"))
}

// chooseNewItemType asks which item type to create, then opens the editor.
func (m *mainWindow) chooseNewItemType() {
	options := make([]string, len(typeOrder))
	for i, t := range typeOrder {
		options[i] = typeSpecs[t].display
	}
	radio := widget.NewRadioGroup(options, nil)
	radio.SetSelected(options[0])

	dialog.ShowCustomConfirm("New item", "Create", "Cancel", radio, func(create bool) {
		if !create || radio.Selected == "" {
			return
		}
		for _, t := range typeOrder {
			if typeSpecs[t].display == radio.Selected {
				m.showEditor(t, nil)
				return
			}
		}
	}, m.win)
}

// confirmDelete asks for confirmation, then deletes the item (revoking any
// outstanding shares) and reloads the list.
func (m *mainWindow) confirmDelete(row itemRow) {
	msg := fmt.Sprintf("Delete %q permanently?\nThis cannot be undone.", row.Title)
	dialog.ShowConfirm("Delete item", msg, func(confirmed bool) {
		if !confirmed {
			return
		}
		m.status.SetText("Deleting…")
		go func() {
			err := m.app.Service.DeleteItem(context.Background(), row.ID)
			fyne.Do(func() {
				if err != nil {
					m.status.SetText(fmt.Sprintf("Error deleting item: %v", err))
					return
				}
				m.setDetail(detailPlaceholder())
				m.reload()
			})
		}()
	}, m.win)
}
