package transfer

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"cowbird/internal/items"
)

// loginSample / cardSample / noteSample are representable in every format.
func loginSample() items.Login {
	return items.Login{
		Title: "Email", Username: "paul@x.test", Password: "hunter2",
		URLs: []string{"https://mail.x.test"}, TOTP: "otpauth://totp/x?secret=ABC",
		Note: "primary", CustomFields: []items.Field{{Type: items.FieldHidden, Label: "recovery", Value: "rk"}},
	}
}

func cardSample() items.Card {
	return items.Card{
		Title: "Visa", Cardholder: "Paul Smith", Number: "4111111111111111",
		ExpirationDate: "08/30", CVV: "123", PIN: "4321", Note: "travel",
	}
}

func noteSample() items.Note {
	return items.Note{Title: "Secret", Body: "the body"}
}

// findLogin / findCard / findNote pick the first item of a kind out of a slice.
func findLogin(t *testing.T, cs []items.Content) items.Login {
	t.Helper()
	for _, c := range cs {
		if v, ok := c.(items.Login); ok {
			return v
		}
	}
	t.Fatalf("no Login in %d items", len(cs))
	return items.Login{}
}

func findCard(t *testing.T, cs []items.Content) items.Card {
	t.Helper()
	for _, c := range cs {
		if v, ok := c.(items.Card); ok {
			return v
		}
	}
	t.Fatalf("no Card in %d items", len(cs))
	return items.Card{}
}

// roundTrip marshals then unmarshals through a codec.
func roundTrip(t *testing.T, c Codec, in []items.Content) []items.Content {
	t.Helper()
	data, err := c.Marshal(in)
	if err != nil {
		t.Fatalf("%s Marshal: %v", c.ID(), err)
	}
	out, skipped, err := c.Unmarshal(data)
	if err != nil {
		t.Fatalf("%s Unmarshal: %v", c.ID(), err)
	}
	if skipped != 0 {
		t.Fatalf("%s round-trip skipped %d", c.ID(), skipped)
	}
	return out
}

func TestRegistry(t *testing.T) {
	for _, id := range []string{"cowbird", "bitwarden", "onepassword", "proton", "lastpass"} {
		if _, ok := Get(id); !ok {
			t.Errorf("codec %q not registered", id)
		}
	}
	if Default().ID() != "cowbird" {
		t.Errorf("Default = %q, want cowbird", Default().ID())
	}
}

func TestBitwardenRoundTrip(t *testing.T) {
	c, _ := Get("bitwarden")
	in := []items.Content{loginSample(), cardSample(), noteSample()}
	out := roundTrip(t, c, in)

	lg := findLogin(t, out)
	if lg.Username != "paul@x.test" || lg.Password != "hunter2" || lg.TOTP == "" {
		t.Errorf("login lost data: %+v", lg)
	}
	if len(lg.URLs) != 1 || lg.URLs[0] != "https://mail.x.test" {
		t.Errorf("login urls: %v", lg.URLs)
	}
	if len(lg.CustomFields) != 1 || lg.CustomFields[0].Type != items.FieldHidden {
		t.Errorf("login custom fields: %+v", lg.CustomFields)
	}
	cd := findCard(t, out)
	if cd.Number != "4111111111111111" || cd.CVV != "123" || cd.ExpirationDate != "08/30" {
		t.Errorf("card lost data: %+v", cd)
	}
	if cd.PIN != "4321" { // PIN has no Bitwarden field; carried as custom and restored
		// PIN becomes a custom field on import (no native field), so it won't be on cd.PIN.
		// Assert it survived somewhere instead.
		if !hasFieldValue(cd.CustomFields, "4321") {
			t.Errorf("card PIN lost: %+v", cd)
		}
	}
}

func TestProtonRoundTrip(t *testing.T) {
	c, _ := Get("proton")
	in := []items.Content{loginSample(), cardSample()}
	out := roundTrip(t, c, in)

	lg := findLogin(t, out)
	if lg.Username != "paul@x.test" || lg.Password != "hunter2" || lg.TOTP == "" {
		t.Errorf("login lost data: %+v", lg)
	}
	cd := findCard(t, out)
	if cd.Number != "4111111111111111" || cd.CVV != "123" || cd.ExpirationDate != "08/30" {
		t.Errorf("card lost data: %+v", cd)
	}
}

func TestOnePasswordRoundTrip(t *testing.T) {
	c, _ := Get("onepassword")
	in := []items.Content{loginSample(), cardSample(), noteSample(),
		items.Identity{Title: "Me", FirstName: "Paul", LastName: "Smith", Email: "p@x.test", JobTitle: "Dev"},
		items.Password{Title: "Wifi", Password: "longpass"}}

	data, err := c.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Produced bytes must be a valid ZIP containing export.data.
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a valid ZIP: %v", err)
	}
	var hasData bool
	for _, f := range zr.File {
		if f.Name == "export.data" {
			hasData = true
		}
	}
	if !hasData {
		t.Fatal("ZIP missing export.data")
	}

	out := roundTrip(t, c, in)
	lg := findLogin(t, out)
	if lg.Username != "paul@x.test" || lg.Password != "hunter2" || lg.TOTP == "" || len(lg.URLs) != 1 {
		t.Errorf("login lost data: %+v", lg)
	}
	cd := findCard(t, out)
	if cd.Cardholder != "Paul Smith" || cd.Number != "4111111111111111" || cd.CVV != "123" ||
		cd.ExpirationDate != "08/30" || cd.PIN != "4321" {
		t.Errorf("card lost data: %+v", cd)
	}
	var id items.Identity
	var pw items.Password
	for _, o := range out {
		switch v := o.(type) {
		case items.Identity:
			id = v
		case items.Password:
			pw = v
		}
	}
	if id.FirstName != "Paul" || id.LastName != "Smith" || id.Email != "p@x.test" || id.JobTitle != "Dev" {
		t.Errorf("identity lost data: %+v", id)
	}
	if pw.Password != "longpass" {
		t.Errorf("password lost data: %+v", pw)
	}
}

func TestLastPassRoundTrip(t *testing.T) {
	c, _ := Get("lastpass")
	in := []items.Content{loginSample(), noteSample()}
	out := roundTrip(t, c, in)

	lg := findLogin(t, out)
	if lg.Username != "paul@x.test" || lg.Password != "hunter2" || len(lg.URLs) != 1 {
		t.Errorf("login lost data: %+v", lg)
	}
	var note items.Note
	for _, o := range out {
		if v, ok := o.(items.Note); ok {
			note = v
		}
	}
	if note.Title != "Secret" || note.Body != "the body" {
		t.Errorf("note lost data: %+v", note)
	}
}

// Cross-format inputs must be rejected (or yield no items), never panic.
func TestCodecsRejectForeignInput(t *testing.T) {
	bw, _ := Get("bitwarden")
	proton, _ := Get("proton")
	op, _ := Get("onepassword")
	lp, _ := Get("lastpass")

	lpCSV := []byte("url,username,password,extra,name\nhttps://x,u,p,,Site\n")

	if _, _, err := bw.Unmarshal(lpCSV); err == nil {
		t.Error("bitwarden accepted CSV input")
	}
	// Proton reads CSV too, but only when a name/title column is present; a CSV
	// without one is rejected rather than producing nameless junk.
	if _, _, err := proton.Unmarshal([]byte("a,b,c\n1,2,3\n")); err == nil {
		t.Error("proton accepted a CSV with no name column")
	}
	if _, _, err := op.Unmarshal([]byte("not a zip")); err == nil {
		t.Error("onepassword accepted non-ZIP input")
	}
	// LastPass parser is lenient but must not panic on JSON; it should error on a
	// header without a name column.
	if _, _, err := lp.Unmarshal([]byte(`{"items":[]}`)); err == nil {
		t.Error("lastpass accepted JSON object as CSV")
	}
}

func TestBitwardenParsesRealSample(t *testing.T) {
	c, _ := Get("bitwarden")
	sample := `{
      "encrypted": false,
      "folders": [],
      "items": [
        {"type":1,"name":"GitHub","notes":"n","login":{"username":"octocat","password":"pw","uris":[{"uri":"https://github.com"}],"totp":"seed"},"fields":[{"name":"PIN","value":"9","type":1}]},
        {"type":3,"name":"Amex","card":{"cardholderName":"P S","number":"3700","code":"1234","expMonth":"5","expYear":"2031"}},
        {"type":4,"name":"ID","identity":{"firstName":"Paul","lastName":"Smith","email":"p@x.test","address1":"1 St","city":"Town"}},
        {"type":2,"name":"NoteItem","notes":"body","secureNote":{"type":0}}
      ]
    }`
	out, skipped, err := c.Unmarshal([]byte(sample))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if skipped != 0 || len(out) != 4 {
		t.Fatalf("got %d items (skipped %d), want 4", len(out), skipped)
	}
	lg := findLogin(t, out)
	if lg.Username != "octocat" || lg.TOTP != "seed" || len(lg.CustomFields) != 1 {
		t.Errorf("login parse: %+v", lg)
	}
	cd := findCard(t, out)
	if cd.ExpirationDate != "05/31" {
		t.Errorf("card expiry parse: %q", cd.ExpirationDate)
	}
}

func TestProtonParsesRealSample(t *testing.T) {
	c, _ := Get("proton")
	sample := `{
      "version":"1.0.0","encrypted":false,"userId":"",
      "vaults":{"v1":{"name":"Personal","description":"","items":[
        {"data":{"metadata":{"name":"Site","note":"n"},"extraFields":[{"fieldName":"sec","type":"hidden","data":{"content":"s"}}],"type":"login","content":{"itemEmail":"","itemUsername":"alice","password":"pw","urls":["https://a.test"],"totpUri":"otpauth://x"}},"state":1,"aliasEmail":null,"contentFormatVersion":6,"createTime":1,"modifyTime":1,"pinned":false}
      ]}}
    }`
	out, skipped, err := c.Unmarshal([]byte(sample))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if skipped != 0 || len(out) != 1 {
		t.Fatalf("got %d items (skipped %d), want 1", len(out), skipped)
	}
	lg := findLogin(t, out)
	if lg.Username != "alice" || lg.Password != "pw" || lg.TOTP != "otpauth://x" || len(lg.URLs) != 1 {
		t.Errorf("login parse: %+v", lg)
	}
	if len(lg.CustomFields) != 1 || lg.CustomFields[0].Type != items.FieldHidden {
		t.Errorf("extra field parse: %+v", lg.CustomFields)
	}
}

func TestProtonParsesCSVSample(t *testing.T) {
	c, _ := Get("proton")
	// Proton Pass CSV export (browser extension). Order-independent header match.
	sample := "type,name,url,email,username,password,note,totp,createTime,modifyTime,vault\n" +
		"login,GitHub,https://github.com,,octocat,pw,a note,otpauth://x,1,1,Personal\n" +
		"login,Bank,https://bank.test,me@x.test,,pw2,,,1,1,Personal\n" +
		"note,Recovery,,,,,the body,,1,1,Personal\n"
	out, skipped, err := c.Unmarshal([]byte(sample))
	if err != nil {
		t.Fatalf("Unmarshal CSV: %v", err)
	}
	if skipped != 0 || len(out) != 3 {
		t.Fatalf("got %d items (skipped %d), want 3", len(out), skipped)
	}
	lg := findLogin(t, out)
	if lg.Title != "GitHub" || lg.Username != "octocat" || lg.Password != "pw" ||
		lg.TOTP != "otpauth://x" || lg.Note != "a note" || len(lg.URLs) != 1 {
		t.Errorf("login row parse: %+v", lg)
	}
	// Second login has only email → used as username.
	var bank items.Login
	for _, o := range out {
		if v, ok := o.(items.Login); ok && v.Title == "Bank" {
			bank = v
		}
	}
	if bank.Username != "me@x.test" {
		t.Errorf("email-only login username: %q", bank.Username)
	}
	var note items.Note
	for _, o := range out {
		if v, ok := o.(items.Note); ok {
			note = v
		}
	}
	if note.Title != "Recovery" || note.Body != "the body" {
		t.Errorf("note row parse: %+v", note)
	}
}

func TestLastPassParsesRealSample(t *testing.T) {
	c, _ := Get("lastpass")
	sample := "url,username,password,totp,extra,name,grouping,fav\n" +
		"https://x.test,bob,pw,,notes here,Site,Folder,0\n" +
		"http://sn,,,,My note body,SecureNote,,0\n"
	out, skipped, err := c.Unmarshal([]byte(sample))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if skipped != 0 || len(out) != 2 {
		t.Fatalf("got %d items (skipped %d), want 2", len(out), skipped)
	}
	lg := findLogin(t, out)
	if lg.Username != "bob" || lg.Note != "notes here" {
		t.Errorf("login parse: %+v", lg)
	}
	var note items.Note
	for _, o := range out {
		if v, ok := o.(items.Note); ok {
			note = v
		}
	}
	if note.Title != "SecureNote" || note.Body != "My note body" {
		t.Errorf("note parse: %+v", note)
	}
}

func TestLastPassRejectsNameOnlyCSV(t *testing.T) {
	c, _ := Get("lastpass")
	// A generic CSV that merely has a "name" column is not a LastPass export and
	// must be rejected rather than silently producing garbage items.
	generic := []byte("id,name,value\n1,thing,42\n")
	if _, _, err := c.Unmarshal(generic); err == nil {
		t.Error("lastpass accepted a non-LastPass CSV that only has a name column")
	}
	// A header missing totp (older LastPass exports) is still accepted.
	older := []byte("url,username,password,extra,name,grouping,fav\nhttps://x,u,p,,Site,F,0\n")
	out, _, err := c.Unmarshal(older)
	if err != nil {
		t.Fatalf("rejected a LastPass CSV lacking the optional totp column: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d items, want 1", len(out))
	}
}

func hasFieldValue(fields []items.Field, value string) bool {
	for _, f := range fields {
		if strings.TrimSpace(f.Value) == value {
			return true
		}
	}
	return false
}
