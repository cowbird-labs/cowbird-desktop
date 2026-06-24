package organization

import (
	"testing"
)

func TestFavoriteToggle(t *testing.T) {
	o := New()
	if o.IsFavorite("a") {
		t.Fatal("new item should not be favorite")
	}
	if !o.ToggleFavorite("a") {
		t.Fatal("toggle should turn favorite on")
	}
	if !o.IsFavorite("a") {
		t.Fatal("IsFavorite should report on")
	}
	if o.ToggleFavorite("a") {
		t.Fatal("toggle should turn favorite off")
	}
	if _, ok := o.Items["a"]; ok {
		t.Fatal("empty meta should be dropped from the map")
	}
}

func TestAssignDedupesAndRequiresDefinition(t *testing.T) {
	o := New()
	// Assigning an undefined label is a no-op.
	o.AssignLabel("item", "ghost")
	if len(o.LabelsOf("item")) != 0 {
		t.Fatal("undefined label should not be assigned")
	}

	work, _ := o.AddLabel("work", "")
	o.AssignLabel("item", work.ID)
	o.AssignLabel("item", work.ID) // duplicate
	if got := o.LabelsOf("item"); len(got) != 1 || got[0] != work.ID {
		t.Fatalf("expected one assignment, got %v", got)
	}

	o.UnassignLabel("item", work.ID)
	if len(o.LabelsOf("item")) != 0 {
		t.Fatal("unassign should remove the label")
	}
	if _, ok := o.Items["item"]; ok {
		t.Fatal("emptied meta should be dropped")
	}
}

func TestDeleteLabelStripsAssignments(t *testing.T) {
	o := New()
	work, _ := o.AddLabel("work", "#fff")
	email, _ := o.AddLabel("email", "")
	o.AssignLabel("a", work.ID)
	o.AssignLabel("a", email.ID)
	o.AssignLabel("b", work.ID)

	o.DeleteLabel(work.ID)

	if _, ok := o.Label(work.ID); ok {
		t.Fatal("label definition should be gone")
	}
	if got := o.LabelsOf("a"); len(got) != 1 || got[0] != email.ID {
		t.Fatalf("item a should retain only email, got %v", got)
	}
	if len(o.LabelsOf("b")) != 0 {
		t.Fatal("item b should have no labels left")
	}
	if _, ok := o.Items["b"]; ok {
		t.Fatal("item b meta should be dropped once empty")
	}
}

func TestRenameRecolor(t *testing.T) {
	o := New()
	l, _ := o.AddLabel("wrok", "")
	if !o.RenameLabel(l.ID, "work") {
		t.Fatal("rename should find the label")
	}
	if !o.RecolorLabel(l.ID, "#123456") {
		t.Fatal("recolor should find the label")
	}
	got, _ := o.Label(l.ID)
	if got.Name != "work" || got.Color != "#123456" {
		t.Fatalf("unexpected label after edits: %+v", got)
	}
	if o.RenameLabel("missing", "x") {
		t.Fatal("rename of missing label should report false")
	}
}

func TestForgetAndPrune(t *testing.T) {
	o := New()
	work, _ := o.AddLabel("work", "")
	o.SetFavorite("keep", true)
	o.AssignLabel("keep", work.ID)
	o.SetFavorite("gone", true)

	o.Forget("gone")
	if _, ok := o.Items["gone"]; ok {
		t.Fatal("Forget should drop the item meta")
	}

	o.SetFavorite("stale", true)
	changed := o.Prune(map[string]bool{"keep": true})
	if !changed {
		t.Fatal("Prune should report a change")
	}
	if _, ok := o.Items["stale"]; ok {
		t.Fatal("stale id should be pruned")
	}
	if !o.IsFavorite("keep") {
		t.Fatal("live id should survive prune")
	}
	if o.Prune(map[string]bool{"keep": true}) {
		t.Fatal("second prune should report no change")
	}
}

func TestJSONRoundTripAndEmpty(t *testing.T) {
	o, err := ParseOrganization(nil)
	if err != nil {
		t.Fatalf("ParseOrganization(nil): %v", err)
	}
	if o.Version != SchemaVersion || len(o.Items) != 0 {
		t.Fatalf("empty parse should be a fresh record, got %+v", o)
	}

	work, _ := o.AddLabel("work", "#abc")
	o.SetFavorite("a", true)
	o.AssignLabel("a", work.ID)

	b, err := o.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	back, err := ParseOrganization(b)
	if err != nil {
		t.Fatalf("ParseOrganization: %v", err)
	}
	if !back.IsFavorite("a") {
		t.Fatal("favorite lost in round trip")
	}
	if got := back.LabelsOf("a"); len(got) != 1 || got[0] != work.ID {
		t.Fatalf("labels lost in round trip: %v", got)
	}
	if l, ok := back.Label(work.ID); !ok || l.Name != "work" || l.Color != "#abc" {
		t.Fatalf("label definition lost in round trip: %+v", l)
	}
}

func TestAddLabelRequiresName(t *testing.T) {
	o := New()
	if _, err := o.AddLabel("", ""); err == nil {
		t.Fatal("expected error for empty label name")
	}
}
