package ui

import (
	"context"
	"errors"
	"sort"
	"strings"

	"cowbird/internal/core"
	"cowbird/internal/items"
	"cowbird/internal/sharing"
)

// itemRow is the list view-model: one row per item the user can read.
type itemRow struct {
	ID      string // itemID for owned rows, shareID for shared rows
	Title   string
	Type    items.ItemType
	Shared  bool
	OwnerID string        // set for shared rows
	Content items.Content // nil when Err is set
	Err     error         // decrypt/decode failure → unreadable row
}

// loadRows processes the inbox, then loads and decrypts everything the user
// can read. A row that fails to decrypt becomes an unreadable entry rather
// than sinking the list; a shared link whose envelope is gone (missed revoke)
// is deleted and omitted. Blocks on Vault I/O — never call on the main thread.
func loadRows(ctx context.Context, app *core.App) ([]itemRow, error) {
	if err := app.Service.ProcessInbox(ctx); err != nil {
		return nil, err
	}

	envs, err := app.Service.ListItems(ctx)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	for _, link := range links {
		row := itemRow{ID: link.ShareID, Type: items.ItemType(link.ItemType), Shared: true, OwnerID: link.OwnerID}
		content, err := app.Service.OpenSharedItem(ctx, link)
		switch {
		case errors.Is(err, sharing.ErrNotFound):
			// Dead link from a missed revoke — clean it up and skip the row.
			if err := app.Service.DeleteSharedLink(ctx, link.ShareID); err != nil {
				return nil, err
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

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].Title) < strings.ToLower(rows[j].Title)
	})
	return rows, nil
}

// matchesFilter reports whether a row passes the search string and type filter.
// search is matched case-insensitively against the title; typ empty means all.
func (r itemRow) matchesFilter(search string, typ items.ItemType) bool {
	if typ != "" && r.Type != typ {
		return false
	}
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(r.Title), strings.ToLower(search))
}
