package sharing

import (
	"context"
	"sync"
	"testing"

	"cowbird/internal/crypto"
	"cowbird/internal/items"
)

// --- in-memory Store ---------------------------------------------------------

// sharedState holds storage that multiple users can both read and write to,
// mirroring the Vault paths that are not scoped to a single entity.
type sharedState struct {
	mu         sync.Mutex
	pubkeys    map[string]PublicKeyEntry     // entityID → entry
	shared     map[string]Envelope           // shareID  → Envelope
	sharedVers map[string]int64              // shareID  → version
	inbox      map[string]map[string]Message // recipientID → msgID → Message
}

func newSharedState() *sharedState {
	return &sharedState{
		pubkeys:    make(map[string]PublicKeyEntry),
		shared:     make(map[string]Envelope),
		sharedVers: make(map[string]int64),
		inbox:      make(map[string]map[string]Message),
	}
}

// memStore is an in-memory Store bound to a specific entityID.
type memStore struct {
	entityID string
	state    *sharedState
	mu       sync.Mutex
	ownItems map[string]Envelope
	links    map[string]SharedLink
	records  map[string]ShareRecord
}

func newMemStore(entityID string, state *sharedState) *memStore {
	return &memStore{
		entityID: entityID,
		state:    state,
		ownItems: make(map[string]Envelope),
		links:    make(map[string]SharedLink),
		records:  make(map[string]ShareRecord),
	}
}

func (m *memStore) PutItem(_ context.Context, id string, env Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ownItems[id] = env
	return nil
}

func (m *memStore) GetItem(_ context.Context, id string) (Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	env, ok := m.ownItems[id]
	if !ok {
		return Envelope{}, ErrNotFound
	}
	return env, nil
}

func (m *memStore) DeleteItem(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.ownItems[id]; !ok {
		return ErrNotFound
	}
	delete(m.ownItems, id)
	return nil
}

func (m *memStore) ListItems(_ context.Context) ([]Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Envelope, 0, len(m.ownItems))
	for _, env := range m.ownItems {
		out = append(out, env)
	}
	return out, nil
}

func (m *memStore) GetPublicKey(_ context.Context, entityID string) ([32]byte, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	entry, ok := m.state.pubkeys[entityID]
	if !ok {
		return [32]byte{}, ErrNotFound
	}
	return entry.Pub, nil
}

func (m *memStore) PutPublicKey(_ context.Context, entityID string, pub [32]byte, name string) error {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.pubkeys[entityID] = PublicKeyEntry{EntityID: entityID, Pub: pub, Name: name}
	return nil
}

func (m *memStore) ListPublicKeys(_ context.Context) ([]PublicKeyEntry, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	out := make([]PublicKeyEntry, 0, len(m.state.pubkeys))
	for _, entry := range m.state.pubkeys {
		out = append(out, entry)
	}
	return out, nil
}

func (m *memStore) PutSharedEnvelope(_ context.Context, shareID string, env Envelope) (int64, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.shared[shareID] = env
	m.state.sharedVers[shareID]++
	return m.state.sharedVers[shareID], nil
}

func (m *memStore) GetSharedEnvelope(_ context.Context, _, shareID string) (Envelope, int64, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	env, ok := m.state.shared[shareID]
	if !ok {
		return Envelope{}, 0, ErrNotFound
	}
	return env, m.state.sharedVers[shareID], nil
}

func (m *memStore) DeleteSharedEnvelope(_ context.Context, shareID string) error {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	if _, ok := m.state.shared[shareID]; !ok {
		return ErrNotFound
	}
	delete(m.state.shared, shareID)
	delete(m.state.sharedVers, shareID)
	return nil
}

func (m *memStore) SendMessage(_ context.Context, recipientID, msgID string, msg Message) error {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	if m.state.inbox[recipientID] == nil {
		m.state.inbox[recipientID] = make(map[string]Message)
	}
	m.state.inbox[recipientID][msgID] = msg
	return nil
}

func (m *memStore) ListInboxMessages(_ context.Context) ([]InboxEntry, error) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	msgs := m.state.inbox[m.entityID]
	out := make([]InboxEntry, 0, len(msgs))
	for id, msg := range msgs {
		out = append(out, InboxEntry{ID: id, Msg: msg})
	}
	return out, nil
}

func (m *memStore) DeleteInboxMessage(_ context.Context, msgID string) error {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	inbox := m.state.inbox[m.entityID]
	if _, ok := inbox[msgID]; !ok {
		return ErrNotFound
	}
	delete(inbox, msgID)
	return nil
}

func (m *memStore) PutSharedLink(_ context.Context, link SharedLink) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.links[link.ShareID] = link
	return nil
}

func (m *memStore) GetSharedLink(_ context.Context, shareID string) (SharedLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[shareID]
	if !ok {
		return SharedLink{}, ErrNotFound
	}
	return link, nil
}

func (m *memStore) DeleteSharedLink(_ context.Context, shareID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.links[shareID]; !ok {
		return ErrNotFound
	}
	delete(m.links, shareID)
	return nil
}

func (m *memStore) ListSharedLinks(_ context.Context) ([]SharedLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SharedLink, 0, len(m.links))
	for _, link := range m.links {
		out = append(out, link)
	}
	return out, nil
}

func (m *memStore) PutShareRecord(_ context.Context, rec ShareRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[rec.ShareID] = rec
	return nil
}

func (m *memStore) ListShareRecords(_ context.Context) ([]ShareRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ShareRecord, 0, len(m.records))
	for _, rec := range m.records {
		out = append(out, rec)
	}
	return out, nil
}

func (m *memStore) DeleteShareRecord(_ context.Context, shareID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[shareID]; !ok {
		return ErrNotFound
	}
	delete(m.records, shareID)
	return nil
}

// --- helpers -----------------------------------------------------------------

type testEnv struct {
	ctx      context.Context
	state    *sharedState
	aliceID  string
	bobID    string
	alice    *crypto.Identity
	bob      *crypto.Identity
	aliceSvc *Service
	bobSvc   *Service
	aliceStr *memStore
	bobStr   *memStore
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	alice, err := crypto.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	bob, err := crypto.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	state := newSharedState()
	aliceID := "alice-entity-id"
	bobID := "bob-entity-id"

	// Register both public keys in the shared directory.
	state.pubkeys[aliceID] = PublicKeyEntry{EntityID: aliceID, Pub: alice.EncryptionPub, Name: "Alice"}
	state.pubkeys[bobID] = PublicKeyEntry{EntityID: bobID, Pub: bob.EncryptionPub, Name: "Bob"}

	aliceStr := newMemStore(aliceID, state)
	bobStr := newMemStore(bobID, state)

	return &testEnv{
		ctx:      context.Background(),
		state:    state,
		aliceID:  aliceID,
		bobID:    bobID,
		alice:    alice,
		bob:      bob,
		aliceSvc: NewService(aliceID, alice, aliceStr),
		bobSvc:   NewService(bobID, bob, bobStr),
		aliceStr: aliceStr,
		bobStr:   bobStr,
	}
}

// --- tests -------------------------------------------------------------------

func TestCreateAndOpenOwnItem(t *testing.T) {
	te := newTestEnv(t)
	original := items.Login{Title: "Test", Username: "alice", Password: "s3cr3t"}

	env, err := te.aliceSvc.CreateItem(te.ctx, original)
	if err != nil {
		t.Fatal(err)
	}

	got, err := te.aliceSvc.OpenOwnItem(te.ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	login, ok := got.(items.Login)
	if !ok {
		t.Fatalf("expected items.Login, got %T", got)
	}
	if login.Username != original.Username || login.Password != original.Password {
		t.Fatalf("content mismatch: got %+v", login)
	}
}

func TestShareAndOpenSharedItem(t *testing.T) {
	te := newTestEnv(t)
	original := items.Note{Title: "Secret Note", Body: "Meet me at dawn."}

	// Alice creates and shares an item with Bob.
	env, err := te.aliceSvc.CreateItem(te.ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}

	// Bob processes his inbox → SharedLink is written.
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, err := te.bobStr.ListSharedLinks(te.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 shared link, got %d", len(links))
	}

	// Bob reads the shared item.
	got, err := te.bobSvc.OpenSharedItem(te.ctx, links[0])
	if err != nil {
		t.Fatal(err)
	}
	note, ok := got.(items.Note)
	if !ok {
		t.Fatalf("expected items.Note, got %T", got)
	}
	if note.Body != original.Body {
		t.Fatalf("body mismatch: got %q, want %q", note.Body, original.Body)
	}
}

func TestRevokeRemovesAccess(t *testing.T) {
	te := newTestEnv(t)
	env, err := te.aliceSvc.CreateItem(te.ctx, items.Note{Title: "Revoked", Body: "Gone."})
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, _ := te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 1 {
		t.Fatalf("expected 1 link before revoke, got %d", len(links))
	}
	shareID := links[0].ShareID

	// Alice revokes.
	if err := te.aliceSvc.Revoke(te.ctx, shareID, te.bobID); err != nil {
		t.Fatal(err)
	}

	// Bob processes the revoke message → link is removed.
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, _ = te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 0 {
		t.Fatalf("expected 0 links after revoke, got %d", len(links))
	}

	// Shared envelope is gone; Bob can no longer open the item.
	// The envelope lookup fails before the WrappedKey is even consulted.
	_, err = te.bobSvc.OpenSharedItem(te.ctx, SharedLink{
		SharePath: sharePath(te.aliceID, shareID),
	})
	if err == nil {
		t.Fatal("expected error opening revoked item, got nil")
	}
}

func TestProcessInboxIdempotent(t *testing.T) {
	te := newTestEnv(t)
	env, err := te.aliceSvc.CreateItem(te.ctx, items.Password{Title: "WiFi", Password: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}

	// Process once normally.
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}

	// Manually re-inject the same share message with the same or lower version.
	links, _ := te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	shareID := links[0].ShareID
	wkBytes := links[0].WrappedKey

	replayMsg := Message{
		Type:       MessageShare,
		ShareID:    shareID,
		SenderID:   te.aliceID,
		EnvVersion: links[0].EnvVersion, // same version
		Share: &SharePayload{
			SharePath:  sharePath(te.aliceID, shareID),
			WrappedKey: wkBytes,
			ItemType:   string(items.TypePassword),
			OwnerID:    te.aliceID,
		},
	}
	replayID := newID()
	te.state.mu.Lock()
	if te.state.inbox[te.bobID] == nil {
		te.state.inbox[te.bobID] = make(map[string]Message)
	}
	te.state.inbox[te.bobID][replayID] = replayMsg
	te.state.mu.Unlock()

	// Process again — replay message should be consumed without duplicating the link.
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, _ = te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after replay, got %d", len(links))
	}
}

func TestShareWritesRecordRevokeRemovesIt(t *testing.T) {
	te := newTestEnv(t)
	env, err := te.aliceSvc.CreateItem(te.ctx, items.Note{Title: "Tracked", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}

	recs, err := te.aliceSvc.ListShareRecords(te.ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 share record, got %d", len(recs))
	}
	if recs[0].ItemID != env.ID || recs[0].RecipientID != te.bobID {
		t.Fatalf("record mismatch: %+v", recs[0])
	}

	if err := te.aliceSvc.Revoke(te.ctx, recs[0].ShareID, te.bobID); err != nil {
		t.Fatal(err)
	}
	recs, err = te.aliceSvc.ListShareRecords(te.ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 share records after revoke, got %d", len(recs))
	}

	// Retrying the revoke after everything is gone must not error.
	if err := te.aliceSvc.Revoke(te.ctx, "no-such-share", te.bobID); err != nil {
		t.Fatalf("re-revoke should be idempotent: %v", err)
	}
}

func TestUpdateItemPropagatesToShares(t *testing.T) {
	te := newTestEnv(t)
	env, err := te.aliceSvc.CreateItem(te.ctx, items.Login{Title: "Site", Username: "alice", Password: "old"})
	if err != nil {
		t.Fatal(err)
	}
	oldNonce := append([]byte(nil), env.Nonce...)

	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}

	updated, err := te.aliceSvc.UpdateItem(te.ctx, env.ID, items.Login{Title: "Site", Username: "alice", Password: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.Nonce) == string(oldNonce) {
		t.Fatal("update must use a fresh nonce")
	}

	// Alice reads her own copy back.
	got, err := te.aliceSvc.OpenOwnItem(te.ctx, updated)
	if err != nil {
		t.Fatal(err)
	}
	if got.(items.Login).Password != "new" {
		t.Fatalf("owner copy not updated: %+v", got)
	}

	// Bob sees the edit through his existing link, without re-sharing.
	links, _ := te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	shared, err := te.bobSvc.OpenSharedItem(te.ctx, links[0])
	if err != nil {
		t.Fatal(err)
	}
	if shared.(items.Login).Password != "new" {
		t.Fatalf("recipient did not see the edit: %+v", shared)
	}
}

func TestDeleteItemRevokesShares(t *testing.T) {
	te := newTestEnv(t)
	env, err := te.aliceSvc.CreateItem(te.ctx, items.Note{Title: "Doomed", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}

	if err := te.aliceSvc.DeleteItem(te.ctx, env.ID); err != nil {
		t.Fatal(err)
	}

	// Owner's item, share record, and the shared envelope are all gone.
	if _, err := te.aliceStr.GetItem(te.ctx, env.ID); err != ErrNotFound {
		t.Fatalf("expected item gone, got %v", err)
	}
	recs, _ := te.aliceSvc.ListShareRecords(te.ctx, env.ID)
	if len(recs) != 0 {
		t.Fatalf("expected 0 share records, got %d", len(recs))
	}
	te.state.mu.Lock()
	nShared := len(te.state.shared)
	te.state.mu.Unlock()
	if nShared != 0 {
		t.Fatalf("expected 0 shared envelopes, got %d", nShared)
	}

	// Bob's inbox has a revoke message; processing it removes his link.
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, _ := te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 0 {
		t.Fatalf("expected 0 links after delete, got %d", len(links))
	}
}

func TestDeleteSharedLinkCleansDeadLink(t *testing.T) {
	te := newTestEnv(t)
	env, err := te.aliceSvc.CreateItem(te.ctx, items.Note{Title: "Dead", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, _ := te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	// Simulate a missed revoke: the envelope vanishes but Bob's link remains.
	te.state.mu.Lock()
	delete(te.state.shared, links[0].ShareID)
	te.state.mu.Unlock()

	if _, err := te.bobSvc.OpenSharedItem(te.ctx, links[0]); err == nil {
		t.Fatal("expected error opening dead link")
	}

	if err := te.bobSvc.DeleteSharedLink(te.ctx, links[0].ShareID); err != nil {
		t.Fatal(err)
	}
	links, _ = te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 0 {
		t.Fatalf("expected 0 links after cleanup, got %d", len(links))
	}
	// Cleaning an already-clean link is fine.
	if err := te.bobSvc.DeleteSharedLink(te.ctx, "already-gone"); err != nil {
		t.Fatalf("second delete should be a no-op: %v", err)
	}
}

func TestDirectoryListsNames(t *testing.T) {
	te := newTestEnv(t)
	entries, err := te.aliceSvc.Directory(te.ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]string, len(entries))
	for _, e := range entries {
		names[e.EntityID] = e.Name
	}
	if names[te.aliceID] != "Alice" || names[te.bobID] != "Bob" {
		t.Fatalf("unexpected directory contents: %v", names)
	}
}

func TestRekeyRotatesAndPreservesAccess(t *testing.T) {
	te := newTestEnv(t)
	original := items.Login{Title: "Site", Username: "alice", Password: "s3cr3t"}

	env, err := te.aliceSvc.CreateItem(te.ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	oldCiphertext := append([]byte(nil), env.Ciphertext...)

	// Alice rotates to a fresh keypair.
	newAlice, err := crypto.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Rekey(te.ctx, te.alice.EncryptionPriv, newAlice.EncryptionPriv, newAlice.EncryptionPub); err != nil {
		t.Fatal(err)
	}

	// Content is re-encrypted under a new item key.
	migrated, err := te.aliceStr.GetItem(te.ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(migrated.Ciphertext) == string(oldCiphertext) {
		t.Fatal("rotation must re-encrypt content under a fresh item key")
	}

	// The old key can no longer read it; the new key can.
	if _, err := te.aliceSvc.OpenOwnItem(te.ctx, migrated); err == nil {
		t.Fatal("old key must not decrypt rotated content")
	}
	newAliceSvc := NewService(te.aliceID, newAlice, te.aliceStr)
	got, err := newAliceSvc.OpenOwnItem(te.ctx, migrated)
	if err != nil {
		t.Fatal(err)
	}
	if got.(items.Login).Password != original.Password {
		t.Fatalf("content mismatch after rotation: %+v", got)
	}

	// Bob keeps access without re-sharing — he processes the re-key message.
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, _ := te.bobStr.ListSharedLinks(te.ctx)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	shared, err := te.bobSvc.OpenSharedItem(te.ctx, links[0])
	if err != nil {
		t.Fatal(err)
	}
	if shared.(items.Login).Password != original.Password {
		t.Fatalf("recipient lost access after rotation: %+v", shared)
	}

	// Re-running rotation is idempotent: already-migrated items are not
	// destructively re-encrypted and access holds.
	if err := newAliceSvc.Rekey(te.ctx, te.alice.EncryptionPriv, newAlice.EncryptionPriv, newAlice.EncryptionPub); err != nil {
		t.Fatalf("re-running rotation should be idempotent: %v", err)
	}
	migrated2, _ := te.aliceStr.GetItem(te.ctx, env.ID)
	if _, err := newAliceSvc.OpenOwnItem(te.ctx, migrated2); err != nil {
		t.Fatalf("item unreadable after idempotent re-run: %v", err)
	}
}

func TestRevokeIdempotent(t *testing.T) {
	te := newTestEnv(t)
	env, err := te.aliceSvc.CreateItem(te.ctx, items.Custom{Title: "Thing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := te.aliceSvc.Share(te.ctx, env.ID, te.bobID); err != nil {
		t.Fatal(err)
	}
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}
	links, _ := te.bobStr.ListSharedLinks(te.ctx)
	shareID := links[0].ShareID

	if err := te.aliceSvc.Revoke(te.ctx, shareID, te.bobID); err != nil {
		t.Fatal(err)
	}
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatal(err)
	}

	// Inject a duplicate revoke message.
	dupMsg := Message{Type: MessageRevoke, ShareID: shareID, SenderID: te.aliceID}
	te.state.mu.Lock()
	if te.state.inbox[te.bobID] == nil {
		te.state.inbox[te.bobID] = make(map[string]Message)
	}
	te.state.inbox[te.bobID][newID()] = dupMsg
	te.state.mu.Unlock()

	// Should not error even though the link is already gone.
	if err := te.bobSvc.ProcessInbox(te.ctx); err != nil {
		t.Fatalf("duplicate revoke should not error: %v", err)
	}
}