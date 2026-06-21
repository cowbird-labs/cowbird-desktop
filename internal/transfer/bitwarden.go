package transfer

import (
	"encoding/json"
	"fmt"
	"strings"

	"cowbird/internal/items"
)

// Bitwarden JSON export. Item type codes: 1 login, 2 secureNote, 3 card,
// 4 identity. Custom field types: 0 text, 1 hidden, 2 boolean, 3 linked.
type bwFile struct {
	Encrypted bool       `json:"encrypted"`
	Folders   []struct{} `json:"folders"`
	Items     []bwItem   `json:"items"`
}

type bwItem struct {
	Type       int         `json:"type"`
	Name       string      `json:"name"`
	Notes      string      `json:"notes,omitempty"`
	Favorite   bool        `json:"favorite"`
	Fields     []bwField   `json:"fields,omitempty"`
	Login      *bwLogin    `json:"login,omitempty"`
	Card       *bwCard     `json:"card,omitempty"`
	Identity   *bwIdentity `json:"identity,omitempty"`
	SecureNote *bwSecNote  `json:"secureNote,omitempty"`
}

type bwField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"`
}

type bwLogin struct {
	Username string  `json:"username,omitempty"`
	Password string  `json:"password,omitempty"`
	URIs     []bwURI `json:"uris,omitempty"`
	TOTP     string  `json:"totp,omitempty"`
}

type bwURI struct {
	URI string `json:"uri"`
}

type bwCard struct {
	CardholderName string `json:"cardholderName,omitempty"`
	Brand          string `json:"brand,omitempty"`
	Number         string `json:"number,omitempty"`
	ExpMonth       string `json:"expMonth,omitempty"`
	ExpYear        string `json:"expYear,omitempty"`
	Code           string `json:"code,omitempty"`
}

type bwIdentity struct {
	Title      string `json:"title,omitempty"`
	FirstName  string `json:"firstName,omitempty"`
	MiddleName string `json:"middleName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	Address1   string `json:"address1,omitempty"`
	Address2   string `json:"address2,omitempty"`
	Address3   string `json:"address3,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postalCode,omitempty"`
	Country    string `json:"country,omitempty"`
	Company    string `json:"company,omitempty"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	SSN        string `json:"ssn,omitempty"`
	Username   string `json:"username,omitempty"`
	Passport   string `json:"passportNumber,omitempty"`
	License    string `json:"licenseNumber,omitempty"`
}

type bwSecNote struct {
	Type int `json:"type"`
}

const (
	bwTypeLogin      = 1
	bwTypeSecureNote = 2
	bwTypeCard       = 3
	bwTypeIdentity   = 4
)

type bitwardenCodec struct{}

func (bitwardenCodec) ID() string        { return "bitwarden" }
func (bitwardenCodec) Name() string      { return "Bitwarden (JSON)" }
func (bitwardenCodec) Extension() string { return ".json" }

func (bitwardenCodec) Marshal(contents []items.Content) ([]byte, error) {
	file := bwFile{Folders: []struct{}{}, Items: make([]bwItem, 0, len(contents))}
	for _, c := range contents {
		file.Items = append(file.Items, bwItemFrom(c))
	}
	return json.MarshalIndent(file, "", "  ")
}

func (bitwardenCodec) Unmarshal(data []byte) ([]items.Content, int, error) {
	var file bwFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, 0, fmt.Errorf("parsing Bitwarden export: %w", err)
	}
	if file.Items == nil {
		return nil, 0, fmt.Errorf("not a Bitwarden export (no items array)")
	}
	out := make([]items.Content, 0, len(file.Items))
	skipped := 0
	for _, it := range file.Items {
		c, ok := bwItemTo(it)
		if !ok {
			skipped++
			continue
		}
		out = append(out, c)
	}
	return out, skipped, nil
}

// --- cowbird → Bitwarden -----------------------------------------------------

func bwItemFrom(c items.Content) bwItem {
	it := bwItem{Name: titleOf(c), Notes: noteOf(c)}
	cf := customFieldsOf(c)
	switch v := c.(type) {
	case items.Login:
		it.Type = bwTypeLogin
		it.Login = &bwLogin{Username: v.Username, Password: v.Password, TOTP: v.TOTP}
		for _, u := range v.URLs {
			it.Login.URIs = append(it.Login.URIs, bwURI{URI: u})
		}
	case items.Password:
		// No Bitwarden standalone-password type; carry as a login.
		it.Type = bwTypeLogin
		it.Login = &bwLogin{Password: v.Password}
	case items.Card:
		it.Type = bwTypeCard
		month, year := splitExpiration(v.ExpirationDate)
		it.Card = &bwCard{CardholderName: v.Cardholder, Number: v.Number, Code: v.CVV, ExpMonth: month, ExpYear: year}
		cf = appendIfValue(cf, "PIN", v.PIN, items.FieldHidden)
	case items.Identity:
		it.Type = bwTypeIdentity
		it.Identity = &bwIdentity{
			FirstName: v.FirstName, LastName: v.LastName, Email: v.Email,
			Phone: v.Phone, Company: v.Company, Address1: v.Address,
		}
		cf = appendIfValue(cf, "Job Title", v.JobTitle, items.FieldText)
	case items.Note:
		it.Type = bwTypeSecureNote
		it.SecureNote = &bwSecNote{Type: 0}
	default: // Custom
		it.Type = bwTypeSecureNote
		it.SecureNote = &bwSecNote{Type: 0}
	}
	it.Fields = bwFieldsFrom(cf)
	return it
}

func bwFieldsFrom(fields []items.Field) []bwField {
	if len(fields) == 0 {
		return nil
	}
	out := make([]bwField, 0, len(fields))
	for _, f := range fields {
		t := 0
		if f.Type == items.FieldHidden {
			t = 1
		}
		out = append(out, bwField{Name: f.Label, Value: f.Value, Type: t})
	}
	return out
}

// --- Bitwarden → cowbird -----------------------------------------------------

func bwItemTo(it bwItem) (items.Content, bool) {
	cf := bwFieldsTo(it.Fields)
	switch it.Type {
	case bwTypeLogin:
		lg := bwLogin{}
		if it.Login != nil {
			lg = *it.Login
		}
		urls := make([]string, 0, len(lg.URIs))
		for _, u := range lg.URIs {
			if u.URI != "" {
				urls = append(urls, u.URI)
			}
		}
		return items.Login{
			Title: it.Name, Username: lg.Username, Password: lg.Password,
			URLs: urls, TOTP: lg.TOTP, Note: it.Notes, CustomFields: cf,
		}, true
	case bwTypeCard:
		cd := bwCard{}
		if it.Card != nil {
			cd = *it.Card
		}
		return items.Card{
			Title: it.Name, Cardholder: cd.CardholderName, Number: cd.Number,
			ExpirationDate: joinExpiration(cd.ExpMonth, cd.ExpYear), CVV: cd.Code,
			Note: it.Notes, CustomFields: cf,
		}, true
	case bwTypeIdentity:
		id := bwIdentity{}
		if it.Identity != nil {
			id = *it.Identity
		}
		cf = appendIfValue(cf, "Title", id.Title, items.FieldText)
		cf = appendIfValue(cf, "Middle Name", id.MiddleName, items.FieldText)
		cf = appendIfValue(cf, "SSN", id.SSN, items.FieldHidden)
		cf = appendIfValue(cf, "Username", id.Username, items.FieldText)
		cf = appendIfValue(cf, "Passport Number", id.Passport, items.FieldText)
		cf = appendIfValue(cf, "License Number", id.License, items.FieldText)
		return items.Identity{
			Title: it.Name, FirstName: id.FirstName, LastName: id.LastName,
			Email: id.Email, Phone: id.Phone, Company: id.Company,
			Address: bwJoinAddress(id), Note: it.Notes, CustomFields: cf,
		}, true
	case bwTypeSecureNote:
		return items.Note{Title: it.Name, Body: it.Notes, CustomFields: cf}, true
	default:
		// Unknown type with no usable mapping; skip.
		return nil, false
	}
}

func bwFieldsTo(fields []bwField) []items.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]items.Field, 0, len(fields))
	for _, f := range fields {
		t := items.FieldText
		if f.Type == 1 {
			t = items.FieldHidden
		}
		out = append(out, field(f.Name, f.Value, t))
	}
	return out
}

func bwJoinAddress(id bwIdentity) string {
	parts := []string{id.Address1, id.Address2, id.Address3, id.City, id.State, id.PostalCode, id.Country}
	var nonEmpty []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, ", ")
}
