# Implementation Plan: Third-Party Import/Export Formats

**Branch**: `003-third-party-formats` | **Spec**: [spec.md](./spec.md)

## Architecture

New UI-independent package **`internal/transfer`**, depending only on
`internal/items`. It owns a `Codec` abstraction and one implementation per
format. Crypto/Vault orchestration stays in `core`; the codec only translates
between the foreign bytes and `[]items.Content`.

```go
// internal/transfer/transfer.go
type Codec interface {
    ID() string          // "cowbird", "bitwarden", "onepassword", "proton", "lastpass"
    Name() string        // human label for the UI, e.g. "1Password (.1pux)"
    Extension() string   // default save extension, e.g. ".1pux", ".json", ".csv"
    Marshal(contents []items.Content) ([]byte, error)
    Unmarshal(data []byte) (contents []items.Content, skipped int, err error)
}

func All() []Codec            // ordered: cowbird first, then the four vendors
func Get(id string) (Codec, bool)
```

- `cowbird.go` — wraps the existing `items.EncodeExport`/`DecodeExport` (native
  format stays in `internal/items`; this is a thin Codec over it).
- `bitwarden.go`, `proton.go`, `lastpass.go`, `onepassword.go` — one each.
- Shared helpers in `mapping.go`: split/join `expMonth`/`expYear` ↔ cowbird
  `ExpirationDate`, custom-field type conversions, a `customField` accumulator.

### core changes (`internal/core/transfer.go`)

`ExportItems`/`ImportItems` gain a `transfer.Codec` parameter; the decrypt-own /
create-own orchestration and skip accounting are unchanged.

```go
func (a *App) ExportItems(ctx context.Context, codec transfer.Codec) ([]byte, error)
func (a *App) ImportItems(ctx context.Context, codec transfer.Codec, data []byte) (ImportResult, error)
```

### UI changes (`internal/ui/transfer.go`)

Both dialogs gain a format `widget.Select` (default "Cowbird (JSON)"). Export:
pick format → clear-text warning → save dialog seeded with the codec's
extension → `ExportItems(codec)`. Import: pick format → file-open →
`ImportItems(codec)` → report counts → reload. The clear-text warning fires for
every format.

## Field-mapping tables

Custom-field type mapping (cowbird `FieldType` ↔ foreign):

| cowbird | Bitwarden field type | Proton extraField type | 1Password section value | LastPass |
|---------|----------------------|------------------------|-------------------------|----------|
| text    | 0                    | text                   | `{"string": v}`         | plain    |
| hidden  | 1                    | hidden                 | `{"concealed": v}`      | plain    |
| url     | 0 (text)             | text                   | `{"url": v}`            | plain    |
| totp    | (top-level totp)     | totp                   | `{"totp": v}`           | totp col |

### Bitwarden JSON

Root `{ "encrypted": false, "folders": [], "items": [ ... ] }`. Item fields:
`type` (1 login, 2 secureNote, 3 card, 4 identity), `name`, `notes`,
`favorite`, `fields: [{name, value, type}]`, plus one type block:

- **login**: `login.username`, `login.password`, `login.uris: [{uri}]`,
  `login.totp` ↔ cowbird Login.
- **card**: `card.cardholderName`, `card.number`, `card.code` (CVV),
  `card.expMonth`, `card.expYear`. cowbird `ExpirationDate` ↔ `MM/YY` from
  expMonth/expYear. PIN has no field → custom field.
- **identity**: `firstName`, `lastName`, `email`, `phone`, `company`,
  `address1`, plus title/middleName/etc. cowbird `Address` ↔ `address1`,
  `JobTitle` → custom field (Bitwarden `title` is salutation, not job).
- **secureNote** (`secureNote.type=0`): `notes` ↔ Note.Body / Custom.
- cowbird **Password** → login (password only); **Custom** → secureNote
  carrying its fields. Reverse: login→Login, secureNote→Note. Lossy, documented.

### Proton Pass JSON

Root `{ "version", "encrypted": false, "userId": "", "vaults": { "<id>": {
"name", "description", "display": {...}, "items": [ ... ] } } }`. Item:
`{ "data": { "metadata": {"name","note"}, "type", "content": {...},
"extraFields": [{"fieldName","type","data":{"content"|"totpUri"}}] },
"state": 1, "pinned": false, "aliasEmail": null, "contentFormatVersion": 6,
"createTime", "modifyTime" }`. Content per type:

- **login**: `itemEmail`, `itemUsername`, `password`, `urls: []`, `totpUri`,
  `passkeys: []`. cowbird Login.Username → `itemUsername` (email left blank, or
  used if username looks like an email — keep simple: username→itemUsername).
- **creditCard**: `cardholderName`, `number`, `verificationNumber` (CVV),
  `expirationDate` (`MMYYYY`), `pin`, `cardType`.
- **note**: content empty; text lives in `metadata.note`.
- **identity**: `fullName`, `email`, `phoneNumber`, `company`, etc. (best-effort).
- Export single vault keyed `"cowbird"`.

### 1Password .1pux

ZIP via `archive/zip`. Members: `export.attributes`
(`{version:3, description:"1Password Unencrypted Export", createdAt:<unix>}`)
and `export.data`. `export.data` =
`{ "accounts": [ { "attrs": {...}, "vaults": [ { "attrs": {uuid,name,type:"P"},
"items": [ ... ] } ] } ] }`. Item:
`{ "uuid", "favIndex", "createdAt", "updatedAt", "state":"active",
"categoryUuid", "overview": {"title","url","urls":[{"label","url"}]},
"details": {...} }`.

- categoryUuid: `001` login, `002` card, `003` note, `004` identity, `005`
  password.
- **details.loginFields**: `[{value, name, designation:"username"|"password",
  fieldType:"T"|"P"}]` (logins).
- **details.notesPlain**: notes / Note body.
- **details.password**: standalone Password category value.
- **details.sections**: `[{title, name, fields:[{title, id, value}]}]`; `value`
  is a single-key object (`string`/`concealed`/`url`/`totp`/`monthYear`).
  Card/identity standard data and all cowbird custom fields live here. Known
  section field ids on import: card `cardholder`/`ccnum`/`cvv`/`expiry`/`pin`,
  identity `firstname`/`lastname`/`email`/`telephone`/`company`/`jobtitle`/
  `address`. Unknown section fields → cowbird custom fields.

### LastPass CSV

Header `url,username,password,totp,extra,name,grouping,fav`. Secure notes use
`url = "http://sn"`; `extra` holds the note body (or `NoteType:`-prefixed
structured data, which we read as opaque body). Export:

- Login → row with url/username/password/totp, `extra`=Note, `name`=Title.
- Note → secure-note row (`url=http://sn`, `extra`=Body).
- Card/Identity/Custom → secure-note row with fields flattened into `extra` as
  `Label: Value` lines (lossy). Password → login row (password only).

Import: `url == "http://sn"` → cowbird Note (extra→Body); otherwise → Login.

## Tests

`internal/transfer/*_test.go`, one per codec:
- Round-trip cowbird→foreign→cowbird for every representable type; assert
  standard fields and custom fields survive, and documented-lossy cases collapse
  as specified.
- Unmarshal a hand-written minimal real-world sample per format (a fixture
  string) and assert the resulting `items.Content`.
- Reject wrong/foreign bytes (e.g. Bitwarden codec on a LastPass CSV) without
  panic, returning an error or a skip count.
- `onepassword_test.go` additionally asserts the produced bytes are a valid ZIP
  containing `export.data` and that it re-reads.

`internal/core/transfer_test.go`: extend existing round-trip to run through a
non-native codec (Bitwarden) end-to-end against the in-memory store.

## Build order

1. `transfer` package skeleton + `Codec` + registry + `cowbird` codec; switch
   `core` and `ui` to the codec API (native still default). Green build.
2. `bitwarden` (cleanest mapping) + tests.
3. `proton` + tests.
4. `lastpass` + tests.
5. `onepassword` (.1pux ZIP) + tests.
6. UI format pickers; full `go test ./...` + `go vet`.

## Confirm during implementation

Exact field names to verify against a real sample before finalizing each adapter:
Proton `creditCard`/`identity` content keys and `extraFields.data` shape;
1Password `005` password detail location and card/identity section field `id`s
and `value` object keys. Adapters tolerate unknown keys (→ custom fields) so a
mismatch degrades gracefully rather than corrupting data.
