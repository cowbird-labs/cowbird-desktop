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
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// mainWindow holds the state of the item list / editor window.
type mainWindow struct {
	app *core.App
	win fyne.Window

	rows     []itemRow         // everything loaded, sorted by title
	filtered []itemRow         // rows matching the current search/type filter
	names    map[string]string // entityID → display name, from the directory

	search     *escapableEntry
	typeFilter *widget.Select
	list       *widget.List
	emptyBox   *fyne.Container // shown instead of the list when there are no items
	detail     *fyne.Container // right pane, single slot
	status     *widget.Label
	retryBtn   *widget.Button
	menuBtn    *widget.Button // hamburger; popup menu anchors to it

	detailCleanup func() // stops background work (e.g. TOTP tickers) tied to the current detail pane
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

	newBtn := ttwidget.NewButtonWithIcon("", theme.ContentAddIcon(), nil)
	newBtn.Importance = widget.LowImportance
	newBtn.SetToolTip("Create New item")
	newBtn.OnTapped = dismissTooltipThen(newBtn, m.chooseNewItemType)
	refreshBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), m.reload)
	refreshBtn.Importance = widget.LowImportance
	refreshBtn.SetToolTip("Refresh")
	toolbar := container.NewHBox(newBtn, refreshBtn)

	m.menuBtn = widget.NewButtonWithIcon("", theme.MenuIcon(), m.showMainMenu)
	m.menuBtn.Importance = widget.LowImportance
	topBar := container.NewBorder(nil, nil, toolbar, m.menuBtn)

	split := container.NewHSplit(m.buildListPane(), m.detail)
	split.SetOffset(0.35)

	statusBar := container.NewBorder(nil, nil, nil, m.retryBtn, m.status)
	content := container.NewBorder(topBar, statusBar, nil, nil, split)
	m.win.SetContent(fynetooltip.AddWindowToolTipLayer(content, m.win.Canvas()))

	m.reload()
	return m.win
}

// reload processes the inbox and reloads all items off the main thread, then
// updates the list. Errors land in the status bar with a Retry action.
func (m *mainWindow) reload() {
	m.status.SetText("Loading…")
	m.retryBtn.Hide()

	go func() {
		rows, names, err := loadRows(context.Background(), m.app)
		fyne.Do(func() {
			if err != nil {
				m.status.SetText(fmt.Sprintf("Error loading items: %v", err))
				m.retryBtn.Show()
				return
			}
			m.status.SetText("")
			m.rows = rows
			m.names = names
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

// setDetail swaps the right pane's content. Main thread only. Any cleanups are
// stored and run the next time the pane is replaced, so background work tied to
// the outgoing view (e.g. TOTP refresh tickers) is stopped when it leaves.
func (m *mainWindow) setDetail(o fyne.CanvasObject, cleanups ...func()) {
	if m.detailCleanup != nil {
		m.detailCleanup()
		m.detailCleanup = nil
	}
	if len(cleanups) > 0 {
		m.detailCleanup = func() {
			for _, c := range cleanups {
				if c != nil {
					c()
				}
			}
		}
	}
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
	radio := newEscapableRadioGroup(options, nil)
	radio.SetSelected(options[0])

	d := dialog.NewCustomConfirm("New item", "Create", "Cancel", radio, func(create bool) {
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
	radio.onEscape = d.Hide

	d.Show()
	m.win.Canvas().Focus(radio)
}

// dismissTooltipThen cancels any pending or shown tooltip on b before invoking
// fn. fn opens a dialog; because clicking a button leaves the pointer resting
// on it (no MouseOut fires), the button's delayed tooltip would otherwise try
// to render over the new dialog overlay — which has no tooltip layer — and the
// tooltip library logs "no tool tip layer created for current overlay".
func dismissTooltipThen(b *ttwidget.Button, fn func()) func() {
	return func() {
		b.MouseOut()
		fn()
	}
}

// escapableRadioGroup is a RadioGroup that invokes onEscape when Escape is
// pressed while it has focus. Fyne routes key events only to the focused
// widget and its dialogs do not dismiss on Escape themselves, so the focused
// widget must forward it. The base RadioGroup is not focusable, so this type
// also implements fyne.Focusable purely to receive the Escape key (selection
// is still by tap); it draws no focus indicator.
type escapableRadioGroup struct {
	widget.RadioGroup
	onEscape func()
}

func newEscapableRadioGroup(options []string, changed func(string)) *escapableRadioGroup {
	r := &escapableRadioGroup{}
	r.Options = options
	r.OnChanged = changed
	r.ExtendBaseWidget(r)
	return r
}

func (r *escapableRadioGroup) FocusGained()   {}
func (r *escapableRadioGroup) FocusLost()     {}
func (r *escapableRadioGroup) TypedRune(rune) {}

func (r *escapableRadioGroup) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && r.onEscape != nil {
		r.onEscape()
	}
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
