package core

import (
	"context"
	"testing"

	"cowbird/internal/crypto"
	"cowbird/internal/items"
	"cowbird/internal/sharing"
	"cowbird/internal/transfer"
)

// memItemStore is a minimal sharing.Store that backs only the owned-item paths
// (PutItem / ListItems) used by export and import. Embedding the interface
// satisfies the remaining methods; any unexpected call panics, which is the
// signal that the test is exercising more than it should.
type memItemStore struct {
	sharing.Store
	items map[string]sharing.Envelope
}

func newMemItemStore() *memItemStore {
	return &memItemStore{items: make(map[string]sharing.Envelope)}
}

func (m *memItemStore) PutItem(_ context.Context, id string, env sharing.Envelope) error {
	m.items[id] = env
	return nil
}

func (m *memItemStore) ListItems(_ context.Context) ([]sharing.Envelope, error) {
	out := make([]sharing.Envelope, 0, len(m.items))
	for _, e := range m.items {
		out = append(out, e)
	}
	return out, nil
}

func (m *memItemStore) GetItem(_ context.Context, id string) (sharing.Envelope, error) {
	e, ok := m.items[id]
	if !ok {
		return sharing.Envelope{}, sharing.ErrNotFound
	}
	return e, nil
}

func (m *memItemStore) DeleteItem(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}

// ListShareRecords returns no shares; imported items are never shared, so the
// dedup deletion path that consults them stays a no-op.
func (m *memItemStore) ListShareRecords(_ context.Context) ([]sharing.ShareRecord, error) {
	return nil, nil
}

func newTestApp(t *testing.T, entityID string, store sharing.Store) *App {
	t.Helper()
	id, err := crypto.NewIdentity()
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	return &App{Identity: id, Service: sharing.NewService(entityID, id, store)}
}

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()

	// Source user with a few owned items.
	srcStore := newMemItemStore()
	src := newTestApp(t, "src", srcStore)
	want := []items.Content{
		items.Login{Title: "Email", Username: "u", Password: "p"},
		items.Note{Title: "Note", Body: "body"},
		items.Card{Title: "Card", Cardholder: "c", Number: "4111", ExpirationDate: "01/30"},
	}
	for _, c := range want {
		if _, err := src.Service.CreateItem(ctx, c); err != nil {
			t.Fatalf("CreateItem: %v", err)
		}
	}

	cowbird := transfer.Default()
	data, err := src.ExportItems(ctx, cowbird)
	if err != nil {
		t.Fatalf("ExportItems: %v", err)
	}

	// Destination user (different identity) imports the file.
	dst := newTestApp(t, "dst", newMemItemStore())
	res, err := dst.ImportItems(ctx, cowbird, data)
	if err != nil {
		t.Fatalf("ImportItems: %v", err)
	}
	if res.Imported != len(want) || res.Skipped != 0 {
		t.Fatalf("result = %+v, want Imported=%d Skipped=0", res, len(want))
	}

	// The destination can now read everything it imported.
	got, err := dst.ExportItems(ctx, cowbird)
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	contents, skipped, err := items.DecodeExport(got)
	if err != nil {
		t.Fatalf("DecodeExport: %v", err)
	}
	if skipped != 0 || len(contents) != len(want) {
		t.Fatalf("re-export has %d items (skipped %d), want %d", len(contents), skipped, len(want))
	}
}

// TestImportExportThroughBitwarden runs the full decrypt→marshal→unmarshal→
// encrypt path through a non-native codec end-to-end against the in-memory store.
func TestImportExportThroughBitwarden(t *testing.T) {
	ctx := context.Background()
	bw, _ := transfer.Get("bitwarden")

	src := newTestApp(t, "src", newMemItemStore())
	want := []items.Content{
		items.Login{Title: "Email", Username: "u", Password: "p", URLs: []string{"https://x.test"}},
		items.Card{Title: "Card", Cardholder: "c", Number: "4111", ExpirationDate: "01/30", CVV: "123"},
		items.Note{Title: "Note", Body: "body"},
	}
	for _, c := range want {
		if _, err := src.Service.CreateItem(ctx, c); err != nil {
			t.Fatalf("CreateItem: %v", err)
		}
	}

	data, err := src.ExportItems(ctx, bw)
	if err != nil {
		t.Fatalf("ExportItems(bitwarden): %v", err)
	}

	dst := newTestApp(t, "dst", newMemItemStore())
	res, err := dst.ImportItems(ctx, bw, data)
	if err != nil {
		t.Fatalf("ImportItems(bitwarden): %v", err)
	}
	if res.Imported != len(want) || res.Skipped != 0 {
		t.Fatalf("result = %+v, want Imported=%d Skipped=0", res, len(want))
	}
}

func TestExportSkipsUndecryptableOwnedItem(t *testing.T) {
	ctx := context.Background()
	store := newMemItemStore()
	app := newTestApp(t, "me", store)

	// One genuinely owned, decryptable item.
	if _, err := app.Service.CreateItem(ctx, items.Note{Title: "mine", Body: "b"}); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	// A foreign envelope with no wrapped key for "me" — cannot be decrypted.
	store.items["foreign"] = sharing.Envelope{ID: "foreign", OwnerID: "someone-else", Type: items.TypeLogin}

	data, err := app.ExportItems(ctx, transfer.Default())
	if err != nil {
		t.Fatalf("ExportItems: %v", err)
	}
	contents, _, err := items.DecodeExport(data)
	if err != nil {
		t.Fatalf("DecodeExport: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("exported %d items, want 1 (foreign item should be skipped)", len(contents))
	}
}

func TestRemoveDuplicateItems(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, "me", newMemItemStore())

	// Import the same set twice (mimicking an accidental double-import).
	set := []items.Content{
		items.Login{Title: "Email", Username: "u", Password: "p"},
		items.Note{Title: "Note", Body: "body"},
		items.Card{Title: "Card", Cardholder: "c", Number: "4111", ExpirationDate: "01/30"},
	}
	for round := 0; round < 2; round++ {
		for _, c := range set {
			if _, err := app.Service.CreateItem(ctx, c); err != nil {
				t.Fatalf("CreateItem: %v", err)
			}
		}
	}

	// A genuinely distinct item must survive.
	if _, err := app.Service.CreateItem(ctx, items.Note{Title: "Unique", Body: "keep me"}); err != nil {
		t.Fatalf("CreateItem unique: %v", err)
	}

	// Dry run reports the count without deleting.
	count, err := app.RemoveDuplicateItems(ctx, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if count != len(set) {
		t.Fatalf("dry-run count = %d, want %d", count, len(set))
	}
	if envs, _ := app.Service.ListItems(ctx); len(envs) != 2*len(set)+1 {
		t.Fatalf("dry run deleted items: have %d, want %d", len(envs), 2*len(set)+1)
	}

	// Real run removes the extra copies, keeping one of each plus the unique item.
	removed, err := app.RemoveDuplicateItems(ctx, false)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed != len(set) {
		t.Fatalf("removed = %d, want %d", removed, len(set))
	}
	envs, _ := app.Service.ListItems(ctx)
	if len(envs) != len(set)+1 {
		t.Fatalf("after dedup have %d items, want %d", len(envs), len(set)+1)
	}

	// Running again is a no-op.
	again, err := app.RemoveDuplicateItems(ctx, false)
	if err != nil || again != 0 {
		t.Fatalf("second dedup removed %d (err %v), want 0", again, err)
	}
}

func TestImportRejectsBadFile(t *testing.T) {
	app := newTestApp(t, "me", newMemItemStore())
	if _, err := app.ImportItems(context.Background(), transfer.Default(), []byte("not a cowbird export")); err == nil {
		t.Fatal("expected error importing a non-export file")
	}
}
