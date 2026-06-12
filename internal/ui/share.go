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
				revokeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
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
// and refreshes the access list.
func (m *mainWindow) confirmRevoke(row itemRow, rec sharing.ShareRecord, body *fyne.Container) {
	msg := fmt.Sprintf("Revoke access to %q for %s?", row.Title, m.displayName(rec.RecipientID))
	dialog.ShowConfirm("Revoke access", msg, func(confirmed bool) {
		if !confirmed {
			return
		}
		m.status.SetText("Revoking…")
		go func() {
			err := m.app.Service.Revoke(context.Background(), rec.ShareID, rec.RecipientID)
			fyne.Do(func() {
				if err != nil {
					m.status.SetText(fmt.Sprintf("Error revoking: %v", err))
					return
				}
				m.status.SetText("")
				m.refreshSharingSection(row, body)
			})
		}()
	}, m.win)
}

// showShareDialog offers the eligible recipients (directory minus self minus
// current recipients) and shares the item with the chosen one.
func (m *mainWindow) showShareDialog(row itemRow, current map[string]bool, body *fyne.Container) {
	labels, byLabel := m.eligibleRecipients(current)
	if len(labels) == 0 {
		dialog.ShowInformation("Share", "There are no other users to share this item with.", m.win)
		return
	}

	sel := widget.NewSelect(labels, nil)
	sel.SetSelected(labels[0])

	dialog.ShowCustomConfirm("Share "+row.Title, "Share", "Cancel", sel, func(share bool) {
		if !share || sel.Selected == "" {
			return
		}
		recipientID := byLabel[sel.Selected]
		m.status.SetText("Sharing…")
		go func() {
			err := m.app.Service.Share(context.Background(), row.ID, recipientID)
			fyne.Do(func() {
				if err != nil {
					m.status.SetText(fmt.Sprintf("Error sharing: %v", err))
					return
				}
				m.status.SetText("")
				m.refreshSharingSection(row, body)
			})
		}()
	}, m.win)
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
