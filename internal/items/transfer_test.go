package items

import (
	"encoding/json"
	"testing"
)

func sampleContents() []Content {
	return []Content{
		Login{
			Title:    "Email",
			Username: "paul@example.com",
			Password: "hunter2",
			URLs:     []string{"https://mail.example.com"},
			TOTP:     "otpauth://totp/x",
			Note:     "primary",
			CustomFields: []Field{
				{Type: FieldHidden, Label: "recovery", Value: "abc"},
			},
		},
		Card{Title: "Visa", Cardholder: "Paul", Number: "4111", ExpirationDate: "01/30", CVV: "123"},
		Note{Title: "Secret", Body: "the body"},
		Identity{Title: "Me", FirstName: "Paul", LastName: "Smith", Email: "p@x.com"},
		Password{Title: "Wifi", Password: "longpassphrase"},
		Custom{Title: "Misc", CustomFields: []Field{{Type: FieldText, Label: "k", Value: "v"}}},
	}
}

func TestExportRoundTrip(t *testing.T) {
	in := sampleContents()

	data, err := EncodeExport(in)
	if err != nil {
		t.Fatalf("EncodeExport: %v", err)
	}

	out, skipped, err := DecodeExport(data)
	if err != nil {
		t.Fatalf("DecodeExport: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(out) != len(in) {
		t.Fatalf("got %d items, want %d", len(out), len(in))
	}
	for i := range in {
		if in[i].Kind() != out[i].Kind() {
			t.Errorf("item %d: kind %s != %s", i, out[i].Kind(), in[i].Kind())
		}
	}
	// Spot-check that field content survived the round-trip.
	login, ok := out[0].(Login)
	if !ok {
		t.Fatalf("item 0 is %T, want Login", out[0])
	}
	if login.Password != "hunter2" || len(login.CustomFields) != 1 || login.CustomFields[0].Value != "abc" {
		t.Errorf("login round-trip lost data: %+v", login)
	}
}

func TestDecodeExportRejectsWrongFormat(t *testing.T) {
	b, _ := json.Marshal(ExportFile{Format: "1password", Version: ExportVersion})
	if _, _, err := DecodeExport(b); err == nil {
		t.Fatal("expected error for wrong format tag")
	}
}

func TestDecodeExportRejectsWrongVersion(t *testing.T) {
	b, _ := json.Marshal(ExportFile{Format: ExportFormat, Version: ExportVersion + 99})
	if _, _, err := DecodeExport(b); err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestDecodeExportRejectsMalformedJSON(t *testing.T) {
	if _, _, err := DecodeExport([]byte("{not json")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestDecodeExportSkipsBadEntry(t *testing.T) {
	good, _ := Encode(Note{Title: "ok", Body: "b"})
	file := ExportFile{
		Format:  ExportFormat,
		Version: ExportVersion,
		Items: []json.RawMessage{
			good,
			json.RawMessage(`{"type":"unknown","data":{}}`),
		},
	}
	b, _ := json.Marshal(file)

	out, skipped, err := DecodeExport(b)
	if err != nil {
		t.Fatalf("DecodeExport: %v", err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(out) != 1 {
		t.Errorf("got %d items, want 1", len(out))
	}
}
