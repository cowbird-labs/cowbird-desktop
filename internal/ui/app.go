package ui

import (
	"context"
	"fmt"
	"time"

	"cowbird/internal/config"
	"cowbird/internal/core"
	"cowbird/internal/items"
	"cowbird/internal/organization"

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
	app  *core.App
	tray *Tray // app-wide system tray; nil when disabled. Re-attached on reconnect.
	win  fyne.Window

	rows     []itemRow         // everything loaded, sorted favorites-first then title
	filtered []itemRow         // rows matching the current search/type filter
	names    map[string]string // entityID → display name, from the directory

	org *organization.Organization // per-user favorites and labels overlay

	search      *escapableEntry
	typeFilter  *widget.Select
	favFilter   *widget.Check
	labelFilter *escapableSelect
	list        *widget.List
	emptyBox    *fyne.Container // shown instead of the list when there are no rows to show
	emptyLabel  *widget.Label   // message inside emptyBox; differs for an empty vault vs. no filter matches
	detail      *fyne.Container // right pane, single slot
	status      *widget.Label
	retryBtn    *widget.Button
	statusBar   *fyne.Container // bottom bar; hidden while there is nothing to show
	menuBtn     *widget.Button  // hamburger; popup menu anchors to it

	detailCleanup func() // stops background work (e.g. TOTP tickers) tied to the current detail pane

	autoLockTimer *time.Timer   // inactivity timer; nil when auto-lock is disabled
	autoLockDur   time.Duration // configured inactivity timeout; 0 disables auto-lock
	clipClearDur  time.Duration // delay before a copied value is wiped; 0 disables clearing

	lastClipboardValue string // last value Cowbird put on the clipboard, for safe clearing
}

// NewMainWindow creates the main item list / editor window.
func NewMainWindow(a fyne.App, app *core.App, tray *Tray) fyne.Window {
	m := &mainWindow{app: app, tray: tray, win: a.NewWindow("Cowbird")}
	m.win.Resize(fyne.NewSize(900, 600))
	m.win.CenterOnScreen()

	// Point the tray (if enabled) at this window so closing it hides to the tray
	// rather than quitting. Nil-safe when the tray is off.
	tray.Attach(m.win)

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
	lockBtn := ttwidget.NewButtonWithIcon("", faIcon("lock.svg"), nil)
	lockBtn.Importance = widget.LowImportance
	lockBtn.SetToolTip("Lock")
	// Lock closes this window; dismiss the tooltip first so its delayed timer
	// doesn't fire against a canvas that no longer exists.
	lockBtn.OnTapped = dismissTooltipThen(lockBtn, m.lock)
	toolbar := container.NewHBox(newBtn, refreshBtn, lockBtn)

	m.menuBtn = widget.NewButtonWithIcon("", theme.MenuIcon(), m.showMainMenu)
	m.menuBtn.Importance = widget.LowImportance
	topBar := container.NewBorder(nil, nil, toolbar, m.menuBtn)

	split := container.NewHSplit(m.buildListPane(), m.detail)
	split.SetOffset(0.35)

	m.statusBar = container.NewBorder(nil, nil, nil, m.retryBtn, m.status)
	m.statusBar.Hide() // collapses the bottom border until a message needs it
	content := container.NewBorder(topBar, m.statusBar, nil, nil, split)
	m.win.SetContent(fynetooltip.AddWindowToolTipLayer(content, m.win.Canvas()))

	// Apply auto-lock / clipboard-clearing preferences. On a config read error
	// both stay disabled (zero durations) rather than blocking the window.
	if cfg, err := config.Load(); err == nil {
		m.autoLockDur = autoLockDuration(cfg.UI)
		m.clipClearDur = clipboardClearDuration(cfg.UI)
	}
	m.startAutoLock()

	// Reset the inactivity timer on stray key events. Fyne only routes typed keys
	// to the canvas handler when no widget is focused, so this is a supplement to
	// the explicit noteActivity calls on list/search/menu interactions, not the
	// sole signal.
	m.win.Canvas().SetOnTypedKey(func(*fyne.KeyEvent) { m.noteActivity() })

	m.reload()
	return m.win
}

// setStatus sets the status-bar text and collapses the bar when there is
// nothing to show (empty text and no Retry button), so it does not occupy a row
// at the bottom of the window the rest of the time. Main thread only.
func (m *mainWindow) setStatus(text string) {
	m.status.SetText(text)
	m.refreshStatusBar()
}

// refreshStatusBar shows the bottom bar when it carries a message or the Retry
// button, and hides it otherwise. Border layout gives a hidden child no space,
// so hiding the bar reclaims the row.
func (m *mainWindow) refreshStatusBar() {
	if m.status.Text == "" && !m.retryBtn.Visible() {
		m.statusBar.Hide()
	} else {
		m.statusBar.Show()
	}
}

// reload processes the inbox and reloads all items off the main thread, then
// updates the list. Errors land in the status bar with a Retry action.
func (m *mainWindow) reload() {
	m.setStatus("Loading…")
	m.retryBtn.Hide()

	go func() {
		rows, names, org, err := loadRows(context.Background(), m.app)
		fyne.Do(func() {
			if err != nil {
				m.retryBtn.Show()
				m.setStatus(fmt.Sprintf("Error loading items: %v", err))
				return
			}
			m.setStatus("")
			m.rows = rows
			m.names = names
			m.org = org
			m.refreshLabelFilter()
			m.applyFilter()
		})
	}()
}

// applyFilter rebuilds the filtered rows from the current search string and
// type filter and refreshes the list. Main thread only.
func (m *mainWindow) applyFilter() {
	m.noteActivity()
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
	favOnly := m.favFilter != nil && m.favFilter.Checked
	labelID := m.selectedLabelID()

	m.filtered = m.filtered[:0]
	for _, row := range m.rows {
		if row.matchesFilter(search, typ, favOnly, labelID) {
			m.filtered = append(m.filtered, row)
		}
	}

	m.list.UnselectAll()
	m.list.Refresh()
	switch {
	case len(m.rows) == 0:
		// Truly empty vault: prompt to create the first item.
		m.emptyLabel.SetText("No items yet.\nUse + to create your first item.")
		m.emptyBox.Show()
		m.list.Hide()
	case len(m.filtered) == 0:
		// Items exist but none match the current search/filters: distinct from an
		// empty vault so the user knows to clear filters, not create an item.
		m.emptyLabel.SetText("No items match your search or filters.\nPress Escape to clear them.")
		m.emptyBox.Show()
		m.list.Hide()
	default:
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
		m.setStatus("Deleting…")
		go func() {
			err := m.app.Service.DeleteItem(context.Background(), row.ID)
			fyne.Do(func() {
				if err != nil {
					m.setStatus(fmt.Sprintf("Error deleting item: %v", err))
					return
				}
				m.setDetail(detailPlaceholder())
				m.reload()
			})
		}()
	}, m.win)
}
