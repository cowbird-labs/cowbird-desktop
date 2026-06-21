package core

import (
	"context"
	"fmt"

	"cowbird/internal/items"
	"cowbird/internal/transfer"
)

// ImportResult reports the outcome of ImportItems: how many items were created
// and how many file entries were skipped (undecodable entries, or items the
// store rejected). Skipped entries are non-fatal.
type ImportResult struct {
	Imported int
	Skipped  int
}

// ExportItems decrypts every item the user owns and serializes them into the
// file format described by codec (cowbird-native or a third-party format). Items
// that cannot be decrypted are skipped rather than aborting the export,
// mirroring how the item list tolerates unreadable rows. The returned bytes
// contain secrets in the clear; the caller is responsible for warning the user
// before persisting them.
func (a *App) ExportItems(ctx context.Context, codec transfer.Codec) ([]byte, error) {
	envs, err := a.Service.ListItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing items for export: %w", err)
	}

	contents := make([]items.Content, 0, len(envs))
	for _, env := range envs {
		content, err := a.Service.OpenOwnItem(ctx, env)
		if err != nil {
			// Undecryptable owned item (e.g. an unreadable row); skip it.
			continue
		}
		contents = append(contents, content)
	}

	return codec.Marshal(contents)
}

// ImportItems parses a file in codec's format and creates each item it contains
// as a new owned item encrypted to the importing user's key. The file is fully
// validated before anything is written, so a malformed or mismatched file
// imports nothing. Individual undecodable entries (counted by the codec) and
// items the store refuses are reported as skips; the import does not abort on
// them.
//
// Imported items receive fresh IDs and are owned by the importing user;
// importing the same file twice creates duplicates (no de-duplication in v1).
func (a *App) ImportItems(ctx context.Context, codec transfer.Codec, data []byte) (ImportResult, error) {
	contents, skipped, err := codec.Unmarshal(data)
	if err != nil {
		return ImportResult{}, err
	}

	res := ImportResult{Skipped: skipped}
	for _, content := range contents {
		if _, err := a.Service.CreateItem(ctx, content); err != nil {
			res.Skipped++
			continue
		}
		res.Imported++
	}
	return res, nil
}

// RemoveDuplicateItems finds owned items whose decrypted content is identical to
// an item already seen and, unless dryRun is set, deletes the extra copies,
// keeping one of each. It returns how many duplicate copies were found (dryRun)
// or removed. This is the cleanup for an accidental double-import: equality is
// exact (the full encoded content must match), so distinct items are never
// merged. Items that cannot be decrypted are ignored. Deletion goes through
// Service.DeleteItem, so any shares of a removed copy are revoked too.
func (a *App) RemoveDuplicateItems(ctx context.Context, dryRun bool) (int, error) {
	envs, err := a.Service.ListItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing items: %w", err)
	}

	seen := make(map[string]bool, len(envs))
	count := 0
	for _, env := range envs {
		content, err := a.Service.OpenOwnItem(ctx, env)
		if err != nil {
			continue // undecryptable; leave it alone
		}
		encoded, err := items.Encode(content)
		if err != nil {
			continue
		}
		key := string(encoded)
		if !seen[key] {
			seen[key] = true
			continue
		}
		count++
		if dryRun {
			continue
		}
		if err := a.Service.DeleteItem(ctx, env.ID); err != nil {
			return count, fmt.Errorf("deleting duplicate item %s: %w", env.ID, err)
		}
	}
	return count, nil
}
