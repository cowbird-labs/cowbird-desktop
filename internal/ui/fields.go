package ui

import (
	"strings"

	"cowbird/internal/items"
)

// fieldSpec describes one standard field of an item type: how to present it
// and how to read/write it on the concrete content struct. Content types use
// value receivers, so setters return the modified value.
type fieldSpec struct {
	label     string
	sensitive bool // masked in the detail view, password entry in the editor
	multiline bool
	required  bool
	get       func(items.Content) string
	set       func(items.Content, string) items.Content
}

func (f fieldSpec) req() fieldSpec    { f.required = true; return f }
func (f fieldSpec) secret() fieldSpec { f.sensitive = true; return f }
func (f fieldSpec) multi() fieldSpec  { f.multiline = true; return f }

// field builds a fieldSpec for concrete content type T, hiding the
// items.Content type assertions in one place.
func field[T items.Content](label string, get func(T) string, set func(*T, string)) fieldSpec {
	return fieldSpec{
		label: label,
		get:   func(c items.Content) string { return get(c.(T)) },
		set: func(c items.Content, v string) items.Content {
			t := c.(T)
			set(&t, v)
			return t
		},
	}
}

// typeSpec describes an item type: display name, a zero value to start a new
// editor from, the standard fields in display order (title first), and access
// to the type's custom-field slice. The detail view and the editor are both
// generated from this table, so they cannot drift apart.
type typeSpec struct {
	display   string
	empty     func() items.Content
	fields    []fieldSpec
	getCustom func(items.Content) []items.Field
	setCustom func(items.Content, []items.Field) items.Content
}

func customAccess[T items.Content](get func(T) []items.Field, set func(*T, []items.Field)) (func(items.Content) []items.Field, func(items.Content, []items.Field) items.Content) {
	return func(c items.Content) []items.Field { return get(c.(T)) },
		func(c items.Content, fs []items.Field) items.Content {
			t := c.(T)
			set(&t, fs)
			return t
		}
}

// typeOrder fixes the menu/filter ordering of item types.
var typeOrder = []items.ItemType{
	items.TypeLogin, items.TypeCard, items.TypeNote,
	items.TypeIdentity, items.TypePassword, items.TypeCustom,
}

var typeSpecs = buildTypeSpecs()

func buildTypeSpecs() map[items.ItemType]typeSpec {
	specs := make(map[items.ItemType]typeSpec, len(typeOrder))

	loginCustomGet, loginCustomSet := customAccess(
		func(c items.Login) []items.Field { return c.CustomFields },
		func(c *items.Login, fs []items.Field) { c.CustomFields = fs })
	specs[items.TypeLogin] = typeSpec{
		display: "Login",
		empty:   func() items.Content { return items.Login{} },
		fields: []fieldSpec{
			field("Title", func(c items.Login) string { return c.Title }, func(c *items.Login, v string) { c.Title = v }).req(),
			field("Username", func(c items.Login) string { return c.Username }, func(c *items.Login, v string) { c.Username = v }),
			field("Password", func(c items.Login) string { return c.Password }, func(c *items.Login, v string) { c.Password = v }).secret(),
			field("URLs (one per line)", func(c items.Login) string { return strings.Join(c.URLs, "\n") }, func(c *items.Login, v string) { c.URLs = splitLines(v) }).multi(),
			field("TOTP secret", func(c items.Login) string { return c.TOTP }, func(c *items.Login, v string) { c.TOTP = v }).secret(),
			field("Note", func(c items.Login) string { return c.Note }, func(c *items.Login, v string) { c.Note = v }).multi(),
		},
		getCustom: loginCustomGet,
		setCustom: loginCustomSet,
	}

	cardCustomGet, cardCustomSet := customAccess(
		func(c items.Card) []items.Field { return c.CustomFields },
		func(c *items.Card, fs []items.Field) { c.CustomFields = fs })
	specs[items.TypeCard] = typeSpec{
		display: "Card",
		empty:   func() items.Content { return items.Card{} },
		fields: []fieldSpec{
			field("Title", func(c items.Card) string { return c.Title }, func(c *items.Card, v string) { c.Title = v }).req(),
			field("Cardholder", func(c items.Card) string { return c.Cardholder }, func(c *items.Card, v string) { c.Cardholder = v }),
			field("Number", func(c items.Card) string { return c.Number }, func(c *items.Card, v string) { c.Number = v }).secret(),
			field("Expiration date", func(c items.Card) string { return c.ExpirationDate }, func(c *items.Card, v string) { c.ExpirationDate = v }),
			field("CVV", func(c items.Card) string { return c.CVV }, func(c *items.Card, v string) { c.CVV = v }).secret(),
			field("PIN", func(c items.Card) string { return c.PIN }, func(c *items.Card, v string) { c.PIN = v }).secret(),
			field("Note", func(c items.Card) string { return c.Note }, func(c *items.Card, v string) { c.Note = v }).multi(),
		},
		getCustom: cardCustomGet,
		setCustom: cardCustomSet,
	}

	noteCustomGet, noteCustomSet := customAccess(
		func(c items.Note) []items.Field { return c.CustomFields },
		func(c *items.Note, fs []items.Field) { c.CustomFields = fs })
	specs[items.TypeNote] = typeSpec{
		display: "Note",
		empty:   func() items.Content { return items.Note{} },
		fields: []fieldSpec{
			field("Title", func(c items.Note) string { return c.Title }, func(c *items.Note, v string) { c.Title = v }).req(),
			field("Body", func(c items.Note) string { return c.Body }, func(c *items.Note, v string) { c.Body = v }).multi(),
		},
		getCustom: noteCustomGet,
		setCustom: noteCustomSet,
	}

	identityCustomGet, identityCustomSet := customAccess(
		func(c items.Identity) []items.Field { return c.CustomFields },
		func(c *items.Identity, fs []items.Field) { c.CustomFields = fs })
	specs[items.TypeIdentity] = typeSpec{
		display: "Identity",
		empty:   func() items.Content { return items.Identity{} },
		fields: []fieldSpec{
			field("Title", func(c items.Identity) string { return c.Title }, func(c *items.Identity, v string) { c.Title = v }).req(),
			field("First name", func(c items.Identity) string { return c.FirstName }, func(c *items.Identity, v string) { c.FirstName = v }),
			field("Last name", func(c items.Identity) string { return c.LastName }, func(c *items.Identity, v string) { c.LastName = v }),
			field("Email", func(c items.Identity) string { return c.Email }, func(c *items.Identity, v string) { c.Email = v }),
			field("Phone", func(c items.Identity) string { return c.Phone }, func(c *items.Identity, v string) { c.Phone = v }),
			field("Address", func(c items.Identity) string { return c.Address }, func(c *items.Identity, v string) { c.Address = v }).multi(),
			field("Company", func(c items.Identity) string { return c.Company }, func(c *items.Identity, v string) { c.Company = v }),
			field("Job title", func(c items.Identity) string { return c.JobTitle }, func(c *items.Identity, v string) { c.JobTitle = v }),
			field("Note", func(c items.Identity) string { return c.Note }, func(c *items.Identity, v string) { c.Note = v }).multi(),
		},
		getCustom: identityCustomGet,
		setCustom: identityCustomSet,
	}

	passwordCustomGet, passwordCustomSet := customAccess(
		func(c items.Password) []items.Field { return c.CustomFields },
		func(c *items.Password, fs []items.Field) { c.CustomFields = fs })
	specs[items.TypePassword] = typeSpec{
		display: "Password",
		empty:   func() items.Content { return items.Password{} },
		fields: []fieldSpec{
			field("Title", func(c items.Password) string { return c.Title }, func(c *items.Password, v string) { c.Title = v }).req(),
			field("Password", func(c items.Password) string { return c.Password }, func(c *items.Password, v string) { c.Password = v }).secret(),
			field("Note", func(c items.Password) string { return c.Note }, func(c *items.Password, v string) { c.Note = v }).multi(),
		},
		getCustom: passwordCustomGet,
		setCustom: passwordCustomSet,
	}

	customCustomGet, customCustomSet := customAccess(
		func(c items.Custom) []items.Field { return c.CustomFields },
		func(c *items.Custom, fs []items.Field) { c.CustomFields = fs })
	specs[items.TypeCustom] = typeSpec{
		display: "Custom",
		empty:   func() items.Content { return items.Custom{} },
		fields: []fieldSpec{
			field("Title", func(c items.Custom) string { return c.Title }, func(c *items.Custom, v string) { c.Title = v }).req(),
		},
		getCustom: customCustomGet,
		setCustom: customCustomSet,
	}

	return specs
}

// titleOf returns an item's display title. Title is always the first field.
func titleOf(c items.Content) string {
	return typeSpecs[c.Kind()].fields[0].get(c)
}

// splitLines turns a multiline entry value into a list, dropping blanks.
func splitLines(v string) []string {
	var out []string
	for _, line := range strings.Split(v, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// customFieldKinds maps display names to field types for the editor's kind
// selector, in display order.
var customFieldKinds = []struct {
	display string
	kind    items.FieldType
}{
	{"Text", items.FieldText},
	{"Hidden", items.FieldHidden},
	{"TOTP", items.FieldTOTP},
	{"URL", items.FieldURL},
}

func kindDisplay(ft items.FieldType) string {
	for _, k := range customFieldKinds {
		if k.kind == ft {
			return k.display
		}
	}
	return string(ft)
}

func kindFromDisplay(display string) items.FieldType {
	for _, k := range customFieldKinds {
		if k.display == display {
			return k.kind
		}
	}
	return items.FieldText
}
