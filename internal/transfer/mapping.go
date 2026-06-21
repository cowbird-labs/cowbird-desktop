package transfer

import (
	"fmt"
	"strings"

	"cowbird/internal/items"
)

// titleOf returns an item's title regardless of concrete type.
func titleOf(c items.Content) string {
	switch v := c.(type) {
	case items.Login:
		return v.Title
	case items.Card:
		return v.Title
	case items.Note:
		return v.Title
	case items.Identity:
		return v.Title
	case items.Password:
		return v.Title
	case items.Custom:
		return v.Title
	default:
		return ""
	}
}

// customFieldsOf returns an item's custom fields regardless of concrete type.
func customFieldsOf(c items.Content) []items.Field {
	switch v := c.(type) {
	case items.Login:
		return v.CustomFields
	case items.Card:
		return v.CustomFields
	case items.Note:
		return v.CustomFields
	case items.Identity:
		return v.CustomFields
	case items.Password:
		return v.CustomFields
	case items.Custom:
		return v.CustomFields
	default:
		return nil
	}
}

// noteOf returns the free-text note/body of an item, or "" if it has none.
func noteOf(c items.Content) string {
	switch v := c.(type) {
	case items.Login:
		return v.Note
	case items.Card:
		return v.Note
	case items.Note:
		return v.Body
	case items.Identity:
		return v.Note
	case items.Password:
		return v.Note
	default:
		return ""
	}
}

// field builds a cowbird custom field, defaulting empty labels so a value is
// never silently dropped for want of a label.
func field(label, value string, t items.FieldType) items.Field {
	if label == "" {
		label = "Field"
	}
	return items.Field{Type: t, Label: label, Value: value}
}

// appendIfValue appends a custom field only when value is non-empty, so empty
// source fields do not litter the imported item.
func appendIfValue(fields []items.Field, label, value string, t items.FieldType) []items.Field {
	if value == "" {
		return fields
	}
	return append(fields, field(label, value, t))
}

// splitExpiration parses a cowbird ExpirationDate ("MM/YY" or "MM/YYYY") into a
// numeric month and a four-digit year. Unparseable input yields ("", "").
func splitExpiration(s string) (month, year string) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", ""
	}
	month = strings.TrimSpace(strings.TrimPrefix(parts[0], "0"))
	if month == "" {
		month = "0"
	}
	year = strings.TrimSpace(parts[1])
	if len(year) == 2 {
		year = "20" + year
	}
	return month, year
}

// joinExpiration renders a "MM/YY" expiration from a month and a (2- or 4-digit)
// year. Either part may be empty.
func joinExpiration(month, year string) string {
	month = strings.TrimSpace(month)
	year = strings.TrimSpace(year)
	if month == "" && year == "" {
		return ""
	}
	if len(month) == 1 {
		month = "0" + month
	}
	if len(year) == 4 {
		year = year[2:]
	}
	return fmt.Sprintf("%s/%s", month, year)
}
