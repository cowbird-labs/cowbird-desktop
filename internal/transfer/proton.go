package transfer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"cowbird/internal/items"
)

// Proton Pass JSON export. A single export holds one or more vaults keyed by an
// opaque vault id; each item carries a typed content block plus an extraFields
// array for custom fields. We read every vault and write a single "cowbird"
// vault. Login, note, and creditCard map to native cowbird types; other Proton
// types (identity, alias, …) fall back to a note carrying their fields, which is
// always importable and never drops data.
type protonFile struct {
	Version   string                 `json:"version"`
	Encrypted bool                   `json:"encrypted"`
	UserID    string                 `json:"userId"`
	Vaults    map[string]protonVault `json:"vaults"`
}

type protonVault struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Display     map[string]any `json:"display,omitempty"`
	Items       []protonItem   `json:"items"`
}

type protonItem struct {
	Data                 protonData `json:"data"`
	State                int        `json:"state"`
	AliasEmail           *string    `json:"aliasEmail"`
	ContentFormatVersion int        `json:"contentFormatVersion"`
	CreateTime           int64      `json:"createTime"`
	ModifyTime           int64      `json:"modifyTime"`
	Pinned               bool       `json:"pinned"`
}

type protonData struct {
	Metadata    protonMeta      `json:"metadata"`
	ExtraFields []protonExtra   `json:"extraFields"`
	Type        string          `json:"type"`
	Content     json.RawMessage `json:"content"`
}

type protonMeta struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

type protonExtra struct {
	FieldName string          `json:"fieldName"`
	Type      string          `json:"type"`
	Data      protonExtraData `json:"data"`
}

type protonExtraData struct {
	Content string `json:"content,omitempty"`
	TotpUri string `json:"totpUri,omitempty"`
}

type protonLogin struct {
	ItemEmail    string   `json:"itemEmail"`
	ItemUsername string   `json:"itemUsername"`
	Username     string   `json:"username,omitempty"` // legacy (contentFormatVersion < 6)
	Password     string   `json:"password"`
	URLs         []string `json:"urls"`
	TotpUri      string   `json:"totpUri"`
	Passkeys     []any    `json:"passkeys"`
}

type protonCard struct {
	CardholderName     string `json:"cardholderName"`
	CardType           int    `json:"cardType"`
	Number             string `json:"number"`
	VerificationNumber string `json:"verificationNumber"`
	ExpirationDate     string `json:"expirationDate"`
	Pin                string `json:"pin"`
}

type protonCodec struct{}

func (protonCodec) ID() string        { return "proton" }
func (protonCodec) Name() string      { return "Proton Pass (JSON or CSV)" }
func (protonCodec) Extension() string { return ".json" }

func (protonCodec) Marshal(contents []items.Content) ([]byte, error) {
	vault := protonVault{Name: "cowbird", Items: make([]protonItem, 0, len(contents))}
	for _, c := range contents {
		it, err := protonItemFrom(c)
		if err != nil {
			return nil, err
		}
		vault.Items = append(vault.Items, it)
	}
	file := protonFile{
		Version: "1.0.0",
		Vaults:  map[string]protonVault{"cowbird": vault},
	}
	return json.MarshalIndent(file, "", "  ")
}

func (protonCodec) Unmarshal(data []byte) ([]items.Content, int, error) {
	// Proton Pass exports either JSON or CSV; dispatch on the first non-space
	// byte ('{' starts the JSON document, anything else is treated as CSV).
	if trimmed := bytes.TrimLeft(data, " \t\r\n\ufeff"); len(trimmed) == 0 || trimmed[0] != '{' {
		return protonUnmarshalCSV(data)
	}

	var file protonFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, 0, fmt.Errorf("parsing Proton Pass export: %w", err)
	}
	if file.Vaults == nil {
		return nil, 0, fmt.Errorf("not a Proton Pass export (no vaults)")
	}
	var out []items.Content
	skipped := 0
	for _, vault := range file.Vaults {
		for _, it := range vault.Items {
			c, ok := protonItemTo(it)
			if !ok {
				skipped++
				continue
			}
			out = append(out, c)
		}
	}
	return out, skipped, nil
}

// protonUnmarshalCSV parses a Proton Pass CSV export. Columns are matched by
// (lower-cased) header name, so column order does not matter; the known headers
// are type,name,url,email,username,password,note,totp. A "note"-type row becomes
// a cowbird Note; everything else becomes a Login (username preferred over email,
// the other kept as a custom field when both are present).
func protonUnmarshalCSV(data []byte) ([]items.Content, int, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // tolerate ragged rows

	header, err := r.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("reading Proton Pass CSV header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.ToLower(strings.Trim(h, "\ufeff")))] = i
	}
	if _, ok := col["name"]; !ok {
		if _, ok := col["title"]; !ok {
			return nil, 0, fmt.Errorf("not a Proton Pass CSV (missing name/title column)")
		}
	}
	get := func(rec []string, keys ...string) string {
		for _, k := range keys {
			if i, ok := col[k]; ok && i < len(rec) {
				if v := rec[i]; v != "" {
					return v
				}
			}
		}
		return ""
	}

	var out []items.Content
	skipped := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			skipped++
			continue
		}
		name := get(rec, "name", "title")
		note := get(rec, "note", "notes")
		switch strings.ToLower(get(rec, "type")) {
		case "note":
			out = append(out, items.Note{Title: name, Body: note})
		default:
			username := get(rec, "username")
			email := get(rec, "email")
			var cf []items.Field
			if username == "" {
				username = email
			} else if email != "" {
				cf = append(cf, field("Email", email, items.FieldText))
			}
			var urls []string
			if u := get(rec, "url"); u != "" {
				urls = []string{u}
			}
			out = append(out, items.Login{
				Title: name, Username: username, Password: get(rec, "password"),
				URLs: urls, TOTP: get(rec, "totp"), Note: note, CustomFields: cf,
			})
		}
	}
	return out, skipped, nil
}

// --- cowbird → Proton --------------------------------------------------------

func protonItemFrom(c items.Content) (protonItem, error) {
	data := protonData{
		Metadata:    protonMeta{Name: titleOf(c), Note: noteOf(c)},
		ExtraFields: protonExtraFrom(customFieldsOf(c)),
	}
	var content any
	switch v := c.(type) {
	case items.Login:
		data.Type = "login"
		content = protonLogin{ItemUsername: v.Username, Password: v.Password, URLs: nonNil(v.URLs), TotpUri: v.TOTP, Passkeys: []any{}}
	case items.Password:
		data.Type = "login"
		content = protonLogin{Password: v.Password, URLs: []string{}, Passkeys: []any{}}
	case items.Card:
		data.Type = "creditCard"
		content = protonCard{
			CardholderName: v.Cardholder, Number: v.Number, VerificationNumber: v.CVV,
			ExpirationDate: protonExpFrom(v.ExpirationDate), Pin: v.PIN,
		}
	case items.Identity:
		// Proton's identity schema is large and version-sensitive; emit a note
		// carrying every field as an extra field so the export stays importable.
		data.Type = "note"
		data.ExtraFields = append(protonIdentityFields(v), data.ExtraFields...)
		content = struct{}{}
	default: // Note, Custom
		data.Type = "note"
		content = struct{}{}
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return protonItem{}, fmt.Errorf("encoding Proton content: %w", err)
	}
	data.Content = raw
	return protonItem{Data: data, State: 1, ContentFormatVersion: 6}, nil
}

func protonExtraFrom(fields []items.Field) []protonExtra {
	out := make([]protonExtra, 0, len(fields))
	for _, f := range fields {
		e := protonExtra{FieldName: f.Label}
		switch f.Type {
		case items.FieldHidden:
			e.Type, e.Data.Content = "hidden", f.Value
		case items.FieldTOTP:
			e.Type, e.Data.TotpUri = "totp", f.Value
		default:
			e.Type, e.Data.Content = "text", f.Value
		}
		out = append(out, e)
	}
	return out
}

func protonIdentityFields(v items.Identity) []protonExtra {
	pairs := []struct {
		label, value string
	}{
		{"First Name", v.FirstName}, {"Last Name", v.LastName}, {"Email", v.Email},
		{"Phone", v.Phone}, {"Address", v.Address}, {"Company", v.Company}, {"Job Title", v.JobTitle},
	}
	var out []protonExtra
	for _, p := range pairs {
		if p.value != "" {
			out = append(out, protonExtra{FieldName: p.label, Type: "text", Data: protonExtraData{Content: p.value}})
		}
	}
	return out
}

// --- Proton → cowbird --------------------------------------------------------

func protonItemTo(it protonItem) (items.Content, bool) {
	name := it.Data.Metadata.Name
	note := it.Data.Metadata.Note
	cf := protonExtraTo(it.Data.ExtraFields)

	switch it.Data.Type {
	case "login":
		var lg protonLogin
		_ = json.Unmarshal(it.Data.Content, &lg)
		username := lg.ItemUsername
		if username == "" {
			username = lg.Username // legacy
		}
		if username == "" {
			username = lg.ItemEmail
		}
		return items.Login{
			Title: name, Username: username, Password: lg.Password,
			URLs: trimEmpty(lg.URLs), TOTP: lg.TotpUri, Note: note, CustomFields: cf,
		}, true
	case "creditCard":
		var cd protonCard
		_ = json.Unmarshal(it.Data.Content, &cd)
		return items.Card{
			Title: name, Cardholder: cd.CardholderName, Number: cd.Number,
			ExpirationDate: protonExpTo(cd.ExpirationDate), CVV: cd.VerificationNumber,
			PIN: cd.Pin, Note: note, CustomFields: cf,
		}, true
	default:
		// note, identity, alias, and anything else: a note carrying the text and
		// any extra fields. Alias e-mail, if present, is preserved as a field.
		if it.AliasEmail != nil && *it.AliasEmail != "" {
			cf = append(cf, field("Alias Email", *it.AliasEmail, items.FieldText))
		}
		return items.Note{Title: name, Body: note, CustomFields: cf}, true
	}
}

func protonExtraTo(extras []protonExtra) []items.Field {
	if len(extras) == 0 {
		return nil
	}
	out := make([]items.Field, 0, len(extras))
	for _, e := range extras {
		switch e.Type {
		case "hidden":
			out = append(out, field(e.FieldName, e.Data.Content, items.FieldHidden))
		case "totp":
			out = append(out, field(e.FieldName, e.Data.TotpUri, items.FieldTOTP))
		default:
			out = append(out, field(e.FieldName, e.Data.Content, items.FieldText))
		}
	}
	return out
}

// protonExpFrom converts cowbird "MM/YY" to Proton's "YYYY-MM".
func protonExpFrom(s string) string {
	month, year := splitExpiration(s)
	if month == "" || year == "" {
		return ""
	}
	if len(month) == 1 {
		month = "0" + month
	}
	return year + "-" + month
}

// protonExpTo converts Proton's "YYYY-MM" (lenient: also accepts "MMYYYY" and
// "MM/YY") to cowbird "MM/YY".
func protonExpTo(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case strings.Contains(s, "-"):
		parts := strings.SplitN(s, "-", 2)
		if len(parts) == 2 {
			return joinExpiration(parts[1], parts[0])
		}
	case strings.Contains(s, "/"):
		return s
	case len(s) == 6: // MMYYYY
		return joinExpiration(s[:2], s[2:])
	}
	return s
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func trimEmpty(s []string) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
