// Package organization models a user's private, per-item organization overlay:
// favorites and label assignments that apply to items the user owns as well as
// items shared with them. It is UI- and Vault-independent so the planned CLI can
// reuse it; callers (internal/core) handle encryption (crypto.SealToSelf) and
// persistence (the users/<entityID>/organization Vault path).
//
// Organization is keyed by item identifier: the itemID for owned items and the
// shareID for items shared with the user. It is never stored in item content and
// never travels in a shared envelope, so toggling a favorite or label never
// rewrites or re-distributes an item and stays private to the user.
package organization

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// SchemaVersion is the current Organization schema version.
const SchemaVersion = 1

// Label is a user-defined tag with an opaque ID, a display name, and an optional
// color (hex, e.g. "#3b82f6").
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// ItemMeta is one item's organization: a favorite flag and assigned label IDs.
type ItemMeta struct {
	Favorite bool     `json:"favorite,omitempty"`
	Labels   []string `json:"labels,omitempty"`
}

func (m ItemMeta) isEmpty() bool {
	return !m.Favorite && len(m.Labels) == 0
}

// Organization is a user's complete organization record: label definitions plus
// per-item metadata. The zero value is not valid; use New or ParseOrganization.
type Organization struct {
	Version int                 `json:"version"`
	Labels  []Label             `json:"labels,omitempty"`
	Items   map[string]ItemMeta `json:"items,omitempty"`
}

// New returns an empty organization record at the current schema version.
func New() *Organization {
	return &Organization{Version: SchemaVersion, Items: map[string]ItemMeta{}}
}

// ParseOrganization decodes a JSON record. Empty input yields a fresh record, so
// an absent Vault entry decodes cleanly to an empty organization.
func ParseOrganization(b []byte) (*Organization, error) {
	if len(b) == 0 {
		return New(), nil
	}
	var o Organization
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("parsing organization: %w", err)
	}
	if o.Version == 0 {
		o.Version = SchemaVersion
	}
	if o.Items == nil {
		o.Items = map[string]ItemMeta{}
	}
	return &o, nil
}

// JSON serializes the record.
func (o *Organization) JSON() ([]byte, error) {
	return json.Marshal(o)
}

// IsFavorite reports whether the item is favorited.
func (o *Organization) IsFavorite(id string) bool {
	return o.Items[id].Favorite
}

// ToggleFavorite flips the favorite flag for an item and returns the new state.
func (o *Organization) ToggleFavorite(id string) bool {
	m := o.Items[id]
	m.Favorite = !m.Favorite
	o.set(id, m)
	return m.Favorite
}

// SetFavorite sets the favorite flag for an item explicitly.
func (o *Organization) SetFavorite(id string, fav bool) {
	m := o.Items[id]
	m.Favorite = fav
	o.set(id, m)
}

// LabelsOf returns the label IDs assigned to an item (a copy).
func (o *Organization) LabelsOf(id string) []string {
	src := o.Items[id].Labels
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// AssignLabel adds a label to an item. No-op if the label is already assigned or
// the labelID is not a defined label.
func (o *Organization) AssignLabel(id, labelID string) {
	if !o.hasLabel(labelID) {
		return
	}
	m := o.Items[id]
	if slices.Contains(m.Labels, labelID) {
		return
	}
	m.Labels = append(m.Labels, labelID)
	o.set(id, m)
}

// UnassignLabel removes a label from an item.
func (o *Organization) UnassignLabel(id, labelID string) {
	m := o.Items[id]
	out := m.Labels[:0]
	for _, l := range m.Labels {
		if l != labelID {
			out = append(out, l)
		}
	}
	m.Labels = out
	if len(m.Labels) == 0 {
		m.Labels = nil
	}
	o.set(id, m)
}

// AddLabel defines a new label with a generated ID. Name must be non-empty.
func (o *Organization) AddLabel(name, color string) (Label, error) {
	if name == "" {
		return Label{}, errors.New("label name is required")
	}
	l := Label{ID: newID(), Name: name, Color: color}
	o.Labels = append(o.Labels, l)
	return l, nil
}

// RenameLabel changes a label's display name. Reports whether the label existed.
func (o *Organization) RenameLabel(labelID, name string) bool {
	for i := range o.Labels {
		if o.Labels[i].ID == labelID {
			o.Labels[i].Name = name
			return true
		}
	}
	return false
}

// RecolorLabel changes a label's color. Reports whether the label existed.
func (o *Organization) RecolorLabel(labelID, color string) bool {
	for i := range o.Labels {
		if o.Labels[i].ID == labelID {
			o.Labels[i].Color = color
			return true
		}
	}
	return false
}

// DeleteLabel removes a label definition and strips it from every item.
func (o *Organization) DeleteLabel(labelID string) {
	out := o.Labels[:0]
	for _, l := range o.Labels {
		if l.ID != labelID {
			out = append(out, l)
		}
	}
	o.Labels = out
	for id := range o.Items {
		o.UnassignLabel(id, labelID)
	}
}

// Label returns the named label definition and whether it exists.
func (o *Organization) Label(labelID string) (Label, bool) {
	for _, l := range o.Labels {
		if l.ID == labelID {
			return l, true
		}
	}
	return Label{}, false
}

// Forget drops an item's metadata entirely (call when an item is deleted).
func (o *Organization) Forget(id string) {
	delete(o.Items, id)
}

// Prune drops metadata for any item id not present in liveIDs, cleaning up after
// deleted items and dead shares. Returns whether anything was removed.
func (o *Organization) Prune(liveIDs map[string]bool) bool {
	changed := false
	for id := range o.Items {
		if !liveIDs[id] {
			delete(o.Items, id)
			changed = true
		}
	}
	return changed
}

// set stores an item's metadata, dropping the entry entirely when empty so the
// map never accumulates blank records.
func (o *Organization) set(id string, m ItemMeta) {
	if m.isEmpty() {
		delete(o.Items, id)
		return
	}
	if o.Items == nil {
		o.Items = map[string]ItemMeta{}
	}
	o.Items[id] = m
}

func (o *Organization) hasLabel(labelID string) bool {
	_, ok := o.Label(labelID)
	return ok
}

// newID returns a random UUID v4.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
