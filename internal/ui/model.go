package ui

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"

	"cowbird/internal/core"
	"cowbird/internal/items"
	"cowbird/internal/organization"
	"cowbird/internal/sharing"
)

// itemRow is the list view-model: one row per item the user can read.
type itemRow struct {
	ID       string // itemID for owned rows, shareID for shared rows
	Title    string
	Type     items.ItemType
	Shared   bool
	OwnerID  string        // set for shared rows
	Content  items.Content // nil when Err is set
	Err      error         // decrypt/decode failure → unreadable row
	Favorite bool          // from the per-user organization overlay
	Labels   []string      // assigned label IDs, from the overlay
}

// loadRows processes the inbox, then loads and decrypts everything the user can
// read, along with the directory's entityID → display-name map and the user's
// organization overlay (favorites and labels). A row that fails to decrypt
// becomes an unreadable entry rather than sinking the list; a shared link whose
// envelope is gone (missed revoke) is deleted and omitted. Organization metadata
// for items no longer present is pruned and saved back. Blocks on Vault I/O —
// never call on the main thread.
func loadRows(ctx context.Context, app *core.App) ([]itemRow, map[string]string, *organization.Organization, error) {
	if err := app.Service.ProcessInbox(ctx); err != nil {
		return nil, nil, nil, err
	}

	dir, err := app.Service.Directory(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	names := make(map[string]string, len(dir))
	for _, e := range dir {
		names[e.EntityID] = e.Name
	}

	envs, err := app.Service.ListItems(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	rows := make([]itemRow, 0, len(envs))
	for _, env := range envs {
		row := itemRow{ID: env.ID, Type: env.Type}
		if content, err := app.Service.OpenOwnItem(ctx, env); err != nil {
			row.Err = err
		} else {
			row.Content = content
			row.Title = titleOf(content)
		}
		rows = append(rows, row)
	}

	links, err := app.Service.ListSharedLinks(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, link := range links {
		row := itemRow{ID: link.ShareID, Type: items.ItemType(link.ItemType), Shared: true, OwnerID: link.OwnerID}
		content, err := app.Service.OpenSharedItem(ctx, link)
		switch {
		case errors.Is(err, sharing.ErrNotFound):
			// Dead link from a missed revoke — clean it up and skip the row.
			if err := app.Service.DeleteSharedLink(ctx, link.ShareID); err != nil {
				return nil, nil, nil, err
			}
			continue
		case err != nil:
			row.Err = err
		default:
			row.Content = content
			row.Title = titleOf(content)
		}
		rows = append(rows, row)
	}

	org, err := app.LoadOrganization(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	// Lazily drop organization for items that no longer exist (deleted items,
	// dead shares), saving back only when something actually changed.
	liveIDs := make(map[string]bool, len(rows))
	for _, r := range rows {
		liveIDs[r.ID] = true
	}
	if org.Prune(liveIDs) {
		if err := app.SaveOrganization(ctx, org); err != nil {
			return nil, nil, nil, err
		}
	}

	annotateRows(rows, org)
	sortRows(rows)
	return rows, names, org, nil
}

// annotateRows stamps each row with its favorite flag and label assignments from
// the organization overlay. Main thread safe; mutates rows in place.
func annotateRows(rows []itemRow, org *organization.Organization) {
	for i := range rows {
		rows[i].Favorite = org.IsFavorite(rows[i].ID)
		rows[i].Labels = org.LabelsOf(rows[i].ID)
	}
}

// sortRows orders favorites first, then case-insensitively by title.
func sortRows(rows []itemRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Favorite != rows[j].Favorite {
			return rows[i].Favorite // favorites sort ahead
		}
		return strings.ToLower(rows[i].Title) < strings.ToLower(rows[j].Title)
	})
}

// matchesFilter reports whether a row passes the search string, type filter,
// favorites toggle, and label filter (all ANDed). search is matched case-
// insensitively against the title; typ/labelID empty means no constraint.
func (r itemRow) matchesFilter(search string, typ items.ItemType, favOnly bool, labelID string) bool {
	if typ != "" && r.Type != typ {
		return false
	}
	if favOnly && !r.Favorite {
		return false
	}
	if labelID != "" && !slices.Contains(r.Labels, labelID) {
		return false
	}
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(r.Title), strings.ToLower(search))
}
