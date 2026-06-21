package transfer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"cowbird/internal/items"
)

// 1Password .1pux export: a ZIP archive containing export.attributes and
// export.data. export.data nests accounts → vaults → items. Item type is a
// categoryUuid (001 login, 002 card, 003 secure note, 004 identity, 005
// password). Login credentials live in details.loginFields; notes in
// details.notesPlain; the standalone password in details.password; everything
// else (card/identity standard fields and all custom fields) lives in
// details.sections, where each field's value is a single-key object keyed by its
// type ("string", "concealed", "monthYear", "totp", "url", …).
const (
	opCatLogin    = "001"
	opCatCard     = "002"
	opCatNote     = "003"
	opCatIdentity = "004"
	opCatPassword = "005"
)

type opData struct {
	Accounts []opAccount `json:"accounts"`
}

type opAccount struct {
	Attrs  map[string]any `json:"attrs"`
	Vaults []opVault      `json:"vaults"`
}

type opVault struct {
	Attrs map[string]any `json:"attrs"`
	Items []opItem       `json:"items"`
}

type opItem struct {
	UUID         string     `json:"uuid"`
	FavIndex     int        `json:"favIndex"`
	CreatedAt    int64      `json:"createdAt"`
	UpdatedAt    int64      `json:"updatedAt"`
	State        string     `json:"state"`
	CategoryUUID string     `json:"categoryUuid"`
	Overview     opOverview `json:"overview"`
	Details      opDetails  `json:"details"`
}

type opOverview struct {
	Title string   `json:"title"`
	URL   string   `json:"url,omitempty"`
	URLs  []opURL  `json:"urls,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

type opURL struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type opDetails struct {
	LoginFields []opLoginField `json:"loginFields,omitempty"`
	NotesPlain  string         `json:"notesPlain,omitempty"`
	Password    string         `json:"password,omitempty"`
	Sections    []opSection    `json:"sections,omitempty"`
}

type opLoginField struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Designation string `json:"designation"`
	FieldType   string `json:"fieldType"`
}

type opSection struct {
	Title  string    `json:"title"`
	Name   string    `json:"name"`
	Fields []opField `json:"fields"`
}

type opField struct {
	Title string         `json:"title"`
	ID    string         `json:"id"`
	Value map[string]any `json:"value"`
}

// Section field ids reused by both marshal and unmarshal so cowbird round-trips
// cleanly; these match 1Password's own ids where known.
var (
	opCardIDs = []struct{ id, label string }{
		{"cardholder", "Cardholder"}, {"ccnum", "Number"}, {"cvv", "CVV"},
		{"expiry", "Expiration"}, {"pin", "PIN"},
	}
	opIdentityIDs = []struct{ id, label string }{
		{"firstname", "First Name"}, {"lastname", "Last Name"}, {"email", "Email"},
		{"phone", "Phone"}, {"address", "Address"}, {"company", "Company"},
		{"jobtitle", "Job Title"},
	}
)

type onePasswordCodec struct{}

func (onePasswordCodec) ID() string        { return "onepassword" }
func (onePasswordCodec) Name() string      { return "1Password (.1pux)" }
func (onePasswordCodec) Extension() string { return ".1pux" }

func (onePasswordCodec) Marshal(contents []items.Content) ([]byte, error) {
	vault := opVault{
		Attrs: map[string]any{"uuid": "cowbird", "name": "cowbird", "type": "P", "desc": "", "avatar": ""},
		Items: make([]opItem, 0, len(contents)),
	}
	now := time.Now().Unix()
	for i, c := range contents {
		vault.Items = append(vault.Items, opItemFrom(c, i, now))
	}
	data := opData{Accounts: []opAccount{{
		Attrs:  map[string]any{"accountName": "cowbird", "name": "cowbird", "uuid": "cowbird", "email": "", "avatar": "", "domain": ""},
		Vaults: []opVault{vault},
	}}}

	dataJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding 1Password export.data: %w", err)
	}
	attrs, _ := json.Marshal(map[string]any{
		"version":     3,
		"description": "1Password Unencrypted Export",
		"createdAt":   now,
	})

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string][]byte{"export.data": dataJSON, "export.attributes": attrs} {
		w, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("creating %s in .1pux: %w", name, err)
		}
		if _, err := w.Write(body); err != nil {
			return nil, fmt.Errorf("writing %s in .1pux: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalizing .1pux: %w", err)
	}
	return buf.Bytes(), nil
}

func (onePasswordCodec) Unmarshal(data []byte) ([]items.Content, int, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, 0, fmt.Errorf("not a .1pux file (cannot open ZIP): %w", err)
	}
	var raw []byte
	for _, f := range zr.File {
		if f.Name == "export.data" {
			rc, err := f.Open()
			if err != nil {
				return nil, 0, fmt.Errorf("opening export.data: %w", err)
			}
			raw, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, 0, fmt.Errorf("reading export.data: %w", err)
			}
			break
		}
	}
	if raw == nil {
		return nil, 0, fmt.Errorf("not a .1pux file (no export.data)")
	}

	var d opData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, 0, fmt.Errorf("parsing export.data: %w", err)
	}
	var out []items.Content
	skipped := 0
	for _, acc := range d.Accounts {
		for _, v := range acc.Vaults {
			for _, it := range v.Items {
				if it.State == "archived" {
					skipped++
					continue
				}
				out = append(out, opItemTo(it))
			}
		}
	}
	return out, skipped, nil
}

// --- cowbird → 1Password -----------------------------------------------------

func opItemFrom(c items.Content, idx int, now int64) opItem {
	it := opItem{
		UUID: fmt.Sprintf("cowbird-%d", idx), State: "active",
		CreatedAt: now, UpdatedAt: now,
		Overview: opOverview{Title: titleOf(c)},
		Details:  opDetails{NotesPlain: noteOf(c)},
	}
	custom := opSectionFrom(customFieldsOf(c))

	switch v := c.(type) {
	case items.Login:
		it.CategoryUUID = opCatLogin
		it.Details.LoginFields = []opLoginField{
			{Value: v.Username, Name: "username", Designation: "username", FieldType: "T"},
			{Value: v.Password, Name: "password", Designation: "password", FieldType: "P"},
		}
		if v.TOTP != "" {
			custom.Fields = append(custom.Fields, opField{Title: "one-time password", ID: "totp", Value: map[string]any{"totp": v.TOTP}})
		}
		for _, u := range v.URLs {
			it.Overview.URLs = append(it.Overview.URLs, opURL{Label: "website", URL: u})
		}
		if len(v.URLs) > 0 {
			it.Overview.URL = v.URLs[0]
		}
	case items.Password:
		it.CategoryUUID = opCatPassword
		it.Details.Password = v.Password
	case items.Card:
		it.CategoryUUID = opCatCard
		sec := opStandardSection(opCardIDs, map[string]string{
			"cardholder": v.Cardholder, "ccnum": v.Number, "cvv": v.CVV,
			"expiry": opExpFrom(v.ExpirationDate), "pin": v.PIN,
		}, map[string]string{"ccnum": "creditCardNumber", "cvv": "concealed", "expiry": "monthYear", "pin": "concealed"})
		it.Details.Sections = append(it.Details.Sections, sec)
	case items.Identity:
		it.CategoryUUID = opCatIdentity
		sec := opStandardSection(opIdentityIDs, map[string]string{
			"firstname": v.FirstName, "lastname": v.LastName, "email": v.Email,
			"phone": v.Phone, "address": v.Address, "company": v.Company, "jobtitle": v.JobTitle,
		}, nil)
		it.Details.Sections = append(it.Details.Sections, sec)
	default: // Note, Custom
		it.CategoryUUID = opCatNote
	}

	if len(custom.Fields) > 0 {
		it.Details.Sections = append(it.Details.Sections, custom)
	}
	return it
}

// opStandardSection builds the typed section for a card or identity. valueType
// overrides the default "string" value key for specific ids (e.g. "monthYear",
// "concealed", "creditCardNumber"). Empty values are omitted.
func opStandardSection(ids []struct{ id, label string }, values, valueType map[string]string) opSection {
	sec := opSection{Title: "", Name: ""}
	for _, f := range ids {
		val := values[f.id]
		if val == "" {
			continue
		}
		key := "string"
		if vt := valueType[f.id]; vt != "" {
			key = vt
		}
		sec.Fields = append(sec.Fields, opField{Title: f.label, ID: f.id, Value: opValue(key, val)})
	}
	return sec
}

func opValue(key, val string) map[string]any {
	if key == "monthYear" {
		if n, err := strconv.Atoi(val); err == nil {
			return map[string]any{"monthYear": n}
		}
	}
	return map[string]any{key: val}
}

func opSectionFrom(fields []items.Field) opSection {
	sec := opSection{Title: "Custom", Name: "cowbird-custom"}
	for _, f := range fields {
		key := "string"
		switch f.Type {
		case items.FieldHidden:
			key = "concealed"
		case items.FieldTOTP:
			key = "totp"
		case items.FieldURL:
			key = "url"
		}
		sec.Fields = append(sec.Fields, opField{Title: f.Label, ID: f.Label, Value: map[string]any{key: f.Value}})
	}
	return sec
}

// opExpFrom converts cowbird "MM/YY" to 1Password's monthYear "YYYYMM" string.
func opExpFrom(s string) string {
	month, year := splitExpiration(s)
	if month == "" || year == "" {
		return ""
	}
	if len(month) == 1 {
		month = "0" + month
	}
	return year + month
}

// --- 1Password → cowbird -----------------------------------------------------

func opItemTo(it opItem) items.Content {
	title := it.Overview.Title
	note := it.Details.NotesPlain

	switch it.CategoryUUID {
	case opCatLogin:
		var username, password string
		for _, lf := range it.Details.LoginFields {
			switch lf.Designation {
			case "username":
				username = lf.Value
			case "password":
				password = lf.Value
			}
		}
		urls := make([]string, 0, len(it.Overview.URLs))
		for _, u := range it.Overview.URLs {
			if u.URL != "" {
				urls = append(urls, u.URL)
			}
		}
		if len(urls) == 0 && it.Overview.URL != "" {
			urls = append(urls, it.Overview.URL)
		}
		totp, cf := opExtractTOTP(it.Details.Sections)
		return items.Login{
			Title: title, Username: username, Password: password,
			URLs: urls, TOTP: totp, Note: note, CustomFields: cf,
		}
	case opCatPassword:
		return items.Password{Title: title, Password: it.Details.Password, Note: note, CustomFields: opAllCustom(it.Details.Sections)}
	case opCatCard:
		known, cf := opSplitKnown(it.Details.Sections, opCardKnownIDs())
		return items.Card{
			Title: title, Cardholder: known["cardholder"], Number: known["ccnum"],
			CVV: known["cvv"], PIN: known["pin"], ExpirationDate: opExpTo(known["expiry"]),
			Note: note, CustomFields: cf,
		}
	case opCatIdentity:
		known, cf := opSplitKnown(it.Details.Sections, opIdentityKnownIDs())
		return items.Identity{
			Title: title, FirstName: known["firstname"], LastName: known["lastname"],
			Email: known["email"], Phone: known["phone"], Address: known["address"],
			Company: known["company"], JobTitle: known["jobtitle"], Note: note, CustomFields: cf,
		}
	default: // note and anything unrecognized
		return items.Note{Title: title, Body: note, CustomFields: opAllCustom(it.Details.Sections)}
	}
}

func opCardKnownIDs() map[string]bool {
	m := map[string]bool{}
	for _, f := range opCardIDs {
		m[f.id] = true
	}
	return m
}

func opIdentityKnownIDs() map[string]bool {
	m := map[string]bool{}
	for _, f := range opIdentityIDs {
		m[f.id] = true
	}
	return m
}

// opSplitKnown returns the values of known-id fields plus the remaining fields
// as cowbird custom fields.
func opSplitKnown(sections []opSection, known map[string]bool) (map[string]string, []items.Field) {
	values := map[string]string{}
	var custom []items.Field
	for _, sec := range sections {
		for _, f := range sec.Fields {
			if known[f.ID] {
				values[f.ID] = opValueString(f.Value)
				continue
			}
			custom = append(custom, opFieldTo(f))
		}
	}
	return values, custom
}

// opExtractTOTP pulls a TOTP field out of the sections and returns it plus the
// remaining fields as custom fields.
func opExtractTOTP(sections []opSection) (string, []items.Field) {
	var totp string
	var custom []items.Field
	for _, sec := range sections {
		for _, f := range sec.Fields {
			if _, ok := f.Value["totp"]; ok && totp == "" {
				totp = opValueString(f.Value)
				continue
			}
			custom = append(custom, opFieldTo(f))
		}
	}
	return totp, custom
}

func opAllCustom(sections []opSection) []items.Field {
	var custom []items.Field
	for _, sec := range sections {
		for _, f := range sec.Fields {
			custom = append(custom, opFieldTo(f))
		}
	}
	return custom
}

func opFieldTo(f opField) items.Field {
	t := items.FieldText
	switch {
	case hasKey(f.Value, "concealed"):
		t = items.FieldHidden
	case hasKey(f.Value, "totp"):
		t = items.FieldTOTP
	case hasKey(f.Value, "url"):
		t = items.FieldURL
	}
	label := f.Title
	if label == "" {
		label = f.ID
	}
	return field(label, opValueString(f.Value), t)
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

// opValueString extracts the scalar value from a 1Password value object,
// regardless of its type key (string/concealed/url/totp/monthYear/…).
func opValueString(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	// monthYear is numeric; render it back as MM/YY via opExpTo at the call site.
	for _, key := range []string{"string", "concealed", "url", "totp", "creditCardNumber", "email", "phone"} {
		if v, ok := m[key]; ok {
			return fmt.Sprint(v)
		}
	}
	// Fall back to whatever single value is present.
	for _, v := range m {
		switch n := v.(type) {
		case float64:
			return strconv.FormatInt(int64(n), 10)
		default:
			return fmt.Sprint(v)
		}
	}
	return ""
}

// opExpTo converts a 1Password monthYear "YYYYMM" to cowbird "MM/YY".
func opExpTo(s string) string {
	if len(s) == 6 {
		return joinExpiration(s[4:], s[:4])
	}
	return s
}
