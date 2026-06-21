package transfer

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"cowbird/internal/items"
)

// LastPass CSV export. Header: url,username,password,totp,extra,name,grouping,fav.
// Secure notes are encoded with url == "http://sn" and the note body (or
// structured "NoteType:"-prefixed data) in the extra column. LastPass logins
// have no native custom-field facility, so cowbird custom fields and any card /
// identity structure are flattened into the extra column as "Label: Value"
// lines — lossy, and re-imported as a single note body.
const lpNoteURL = "http://sn"

var lpHeader = []string{"url", "username", "password", "totp", "extra", "name", "grouping", "fav"}

type lastPassCodec struct{}

func (lastPassCodec) ID() string        { return "lastpass" }
func (lastPassCodec) Name() string      { return "LastPass (CSV)" }
func (lastPassCodec) Extension() string { return ".csv" }

func (lastPassCodec) Marshal(contents []items.Content) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(lpHeader); err != nil {
		return nil, err
	}
	for _, c := range contents {
		if err := w.Write(lpRowFrom(c)); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("writing LastPass CSV: %w", err)
	}
	return buf.Bytes(), nil
}

func (lastPassCodec) Unmarshal(data []byte) ([]items.Content, int, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // tolerate ragged rows

	header, err := r.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("reading LastPass CSV header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.ToLower(h))] = i
	}
	if _, ok := col["name"]; !ok {
		return nil, 0, fmt.Errorf("not a LastPass CSV (missing name column)")
	}

	get := func(rec []string, key string) string {
		i, ok := col[key]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
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
		out = append(out, lpRowTo(
			get(rec, "url"), get(rec, "username"), get(rec, "password"),
			get(rec, "totp"), get(rec, "extra"), get(rec, "name"),
		))
	}
	return out, skipped, nil
}

// --- cowbird → LastPass ------------------------------------------------------

func lpRowFrom(c items.Content) []string {
	// columns: url, username, password, totp, extra, name, grouping, fav
	row := make([]string, len(lpHeader))
	row[5] = titleOf(c) // name
	row[7] = "0"        // fav

	switch v := c.(type) {
	case items.Login:
		if len(v.URLs) > 0 {
			row[0] = v.URLs[0]
		}
		row[1] = v.Username
		row[2] = v.Password
		row[3] = v.TOTP
		row[4] = lpLoginExtra(v)
	case items.Password:
		row[2] = v.Password
		row[4] = v.Note
	default:
		// Note, Card, Identity, Custom → secure note; fields flattened to extra.
		row[0] = lpNoteURL
		row[4] = lpNoteExtra(c)
	}
	return row
}

// lpLoginExtra builds a login's extra column: its note, then any extra URLs and
// custom fields as labelled lines (LastPass logins cannot carry these natively).
func lpLoginExtra(v items.Login) string {
	var b strings.Builder
	b.WriteString(v.Note)
	for _, u := range v.URLs[min(1, len(v.URLs)):] {
		appendLine(&b, "URL", u)
	}
	for _, f := range v.CustomFields {
		appendLine(&b, f.Label, f.Value)
	}
	return b.String()
}

// lpNoteExtra serializes a non-login item's full content into the extra column.
func lpNoteExtra(c items.Content) string {
	var b strings.Builder
	switch v := c.(type) {
	case items.Note:
		b.WriteString(v.Body)
	case items.Card:
		appendLine(&b, "Cardholder", v.Cardholder)
		appendLine(&b, "Number", v.Number)
		appendLine(&b, "Expiration", v.ExpirationDate)
		appendLine(&b, "CVV", v.CVV)
		appendLine(&b, "PIN", v.PIN)
		appendLine(&b, "Note", v.Note)
	case items.Identity:
		appendLine(&b, "First Name", v.FirstName)
		appendLine(&b, "Last Name", v.LastName)
		appendLine(&b, "Email", v.Email)
		appendLine(&b, "Phone", v.Phone)
		appendLine(&b, "Address", v.Address)
		appendLine(&b, "Company", v.Company)
		appendLine(&b, "Job Title", v.JobTitle)
		appendLine(&b, "Note", v.Note)
	}
	for _, f := range customFieldsOf(c) {
		appendLine(&b, f.Label, f.Value)
	}
	return strings.TrimLeft(b.String(), "\n")
}

func appendLine(b *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(label)
	b.WriteString(": ")
	b.WriteString(value)
}

// --- LastPass → cowbird ------------------------------------------------------

func lpRowTo(url, username, password, totp, extra, name string) items.Content {
	if strings.EqualFold(strings.TrimSpace(url), lpNoteURL) {
		return items.Note{Title: name, Body: extra}
	}
	var urls []string
	if strings.TrimSpace(url) != "" {
		urls = []string{url}
	}
	return items.Login{
		Title: name, Username: username, Password: password,
		URLs: urls, TOTP: totp, Note: extra,
	}
}
