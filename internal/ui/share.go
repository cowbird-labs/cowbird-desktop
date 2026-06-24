package ui

import (
	"context"
	"fmt"
	"sort"

	"cowbird/internal/sharing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// displayName resolves an entity ID to its published display name, falling
// back to an entity-ID prefix for entries without one.
func (m *mainWindow) displayName(entityID string) string {
	if name := m.names[entityID]; name != "" {
		return name
	}
	return idPrefix(entityID)
}

func idPrefix(entityID string) string {
	if len(entityID) > 8 {
		return entityID[:8] + "…"
	}
	return entityID
}

// buildSharingSection returns the "Sharing" block of an owned item's detail
// view. The access list is fetched asynchronously and refreshed in place
// after every share/revoke.
func (m *mainWindow) buildSharingSection(row itemRow) fyne.CanvasObject {
	body := container.NewVBox(widget.NewLabel("Loading…"))
	m.refreshSharingSection(row, body)
	return container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Sharing", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		body,
	)
}

// refreshSharingSection reloads the item's ShareRecords and rebuilds body.
func (m *mainWindow) refreshSharingSection(row itemRow, body *fyne.Container) {
	go func() {
		recs, err := m.app.Service.ListShareRecords(context.Background(), row.ID)
		fyne.Do(func() {
			body.Objects = nil
			if err != nil {
				lbl := widget.NewLabel(fmt.Sprintf("Error loading shares: %v", err))
				lbl.Wrapping = fyne.TextWrapWord
				body.Add(lbl)
				body.Refresh()
				return
			}

			if len(recs) == 0 {
				body.Add(widget.NewLabel("Not shared."))
			}
			current := make(map[string]bool, len(recs))
			for _, rec := range recs {
				current[rec.RecipientID] = true
				rec := rec
				revokeBtn := ttwidget.NewButtonWithIcon("", theme.CancelIcon(), nil)
				revokeBtn.SetToolTip("Remove access")
				revokeBtn.OnTapped = dismissTooltipThen(revokeBtn, func() {
					m.confirmRevoke(row, rec, body)
				})
				body.Add(container.NewBorder(nil, nil, nil, revokeBtn,
					widget.NewLabel(m.displayName(rec.RecipientID))))
			}
			body.Add(widget.NewButtonWithIcon("Share…", theme.AccountIcon(), func() {
				m.showShareDialog(row, current, body)
			}))
			body.Refresh()
		})
	}()
}

// confirmRevoke asks for confirmation, then revokes one recipient's access
// and refreshes the access list. Enter revokes, Escape dismisses.
func (m *mainWindow) confirmRevoke(row itemRow, rec sharing.ShareRecord, body *fyne.Container) {
	revoke := func() {
		m.setStatus("Revoking…")
		go func() {
			err := m.app.Service.Revoke(context.Background(), rec.ShareID, rec.RecipientID)
			fyne.Do(func() {
				if err != nil {
					m.setStatus(fmt.Sprintf("Error revoking: %v", err))
					return
				}
				m.setStatus("")
				m.refreshSharingSection(row, body)
			})
		}()
	}

	var d dialog.Dialog
	// The dialog content is just a message label (not focusable), so a hidden
	// key-catcher carries Enter/Escape — Fyne routes keys only to the focused
	// widget and its dialogs do not handle Escape themselves.
	catcher := newKeyCatcher(
		func() { d.Hide(); revoke() },
		func() { d.Hide() },
	)
	msg := widget.NewLabel(fmt.Sprintf("Revoke access to %q for %s?", row.Title, m.displayName(rec.RecipientID)))
	msg.Wrapping = fyne.TextWrapWord
	content := container.NewStack(msg, catcher)

	d = dialog.NewCustomConfirm("Revoke access", "Revoke", "Cancel", content, func(confirmed bool) {
		if confirmed {
			revoke()
		}
	}, m.win)
	d.Show()
	m.win.Canvas().Focus(catcher)
}

// showShareDialog offers the eligible recipients (directory minus self minus
// current recipients) and shares the item with the chosen one. Enter shares,
// Escape cancels.
func (m *mainWindow) showShareDialog(row itemRow, current map[string]bool, body *fyne.Container) {
	labels, byLabel := m.eligibleRecipients(current)
	if len(labels) == 0 {
		var d dialog.Dialog
		dismiss := func() { d.Hide() }
		catcher := newKeyCatcher(dismiss, dismiss)
		content := container.NewStack(
			widget.NewLabel("There are no other users to share this item with."),
			catcher,
		)
		d = dialog.NewCustom("Share", "OK", content, m.win)
		d.Show()
		m.win.Canvas().Focus(catcher)
		return
	}

	sel := newEscapableSelect(labels, nil)
	sel.SetSelected(labels[0])

	var d dialog.Dialog
	share := func() {
		if sel.Selected == "" {
			return
		}
		recipientID := byLabel[sel.Selected]
		m.setStatus("Sharing…")
		go func() {
			err := m.app.Service.Share(context.Background(), row.ID, recipientID)
			fyne.Do(func() {
				if err != nil {
					m.setStatus(fmt.Sprintf("Error sharing: %v", err))
					return
				}
				m.setStatus("")
				m.refreshSharingSection(row, body)
			})
		}()
	}

	d = dialog.NewCustomConfirm("Share "+row.Title, "Share", "Cancel", sel, func(doShare bool) {
		if doShare {
			share()
		}
	}, m.win)
	sel.onEscape = d.Hide
	sel.onSubmit = func() { d.Hide(); share() }
	d.Show()
	m.win.Canvas().Focus(sel)
}

// keyCatcher is an invisible, zero-size focusable widget that forwards Enter
// and Escape to callbacks. It exists so dialogs whose content is not otherwise
// focusable can still be driven from the keyboard (Fyne routes key events only
// to the focused widget).
type keyCatcher struct {
	widget.BaseWidget
	onEnter  func()
	onEscape func()
}

func newKeyCatcher(onEnter, onEscape func()) *keyCatcher {
	k := &keyCatcher{onEnter: onEnter, onEscape: onEscape}
	k.ExtendBaseWidget(k)
	return k
}

func (k *keyCatcher) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewWithoutLayout())
}

func (k *keyCatcher) FocusGained()   {}
func (k *keyCatcher) FocusLost()     {}
func (k *keyCatcher) TypedRune(rune) {}

func (k *keyCatcher) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if k.onEnter != nil {
			k.onEnter()
		}
	case fyne.KeyEscape:
		if k.onEscape != nil {
			k.onEscape()
		}
	}
}

// escapableSelect is a Select that forwards Enter to onSubmit and Escape to
// onEscape. The base Select ignores Enter, so mapping it to submit does not
// clash with the dropdown's own keys (Space/arrows still open and cycle it).
type escapableSelect struct {
	widget.Select
	onSubmit func()
	onEscape func()
}

func newEscapableSelect(options []string, changed func(string)) *escapableSelect {
	s := &escapableSelect{}
	s.Options = options
	s.OnChanged = changed
	s.ExtendBaseWidget(s)
	return s
}

func (s *escapableSelect) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if s.onSubmit != nil {
			s.onSubmit()
		}
	case fyne.KeyEscape:
		if s.onEscape != nil {
			s.onEscape()
		}
	default:
		s.Select.TypedKey(key)
	}
}

// eligibleRecipients returns sorted picker labels and a label → entityID map
// for every directory entry except the current user and excluded IDs.
// Nameless entries and duplicate names get an entity-ID prefix suffix so
// every label is unambiguous.
func (m *mainWindow) eligibleRecipients(exclude map[string]bool) ([]string, map[string]string) {
	nameCount := make(map[string]int)
	var ids []string
	for id, name := range m.names {
		if id == m.app.Vault.EntityID || exclude[id] {
			continue
		}
		ids = append(ids, id)
		nameCount[name]++
	}

	labels := make([]string, 0, len(ids))
	byLabel := make(map[string]string, len(ids))
	for _, id := range ids {
		name := m.names[id]
		label := name
		if name == "" {
			label = idPrefix(id)
		} else if nameCount[name] > 1 {
			label = fmt.Sprintf("%s (%s)", name, idPrefix(id))
		}
		labels = append(labels, label)
		byLabel[label] = id
	}
	sort.Strings(labels)
	return labels, byLabel
}
