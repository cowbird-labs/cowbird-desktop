package ui

import (
	"embed"
	"path"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// iconFS holds the vendored Font Awesome Free (solid) SVGs that replace Fyne's
// built-in icon set. They are single-path monochrome glyphs, so Fyne's
// ThemedResource recolors them to the active theme colour exactly like the
// stock icons (see replacePathsFill in fyne's internal/svg).
//
// Font Awesome Free icons are licensed CC BY 4.0; see icons/ATTRIBUTION.md.
//
//go:embed icons/*.svg
var iconFS embed.FS

// faIcons maps Fyne theme icon names to the Font Awesome glyph file that should
// stand in for them. Anything not listed here falls back to the default theme,
// so widget internals we deliberately leave alone (checkboxes, radios, the
// colour picker) keep Fyne's own art.
var faIcons = map[fyne.ThemeIconName]string{
	// Actions surfaced as buttons throughout the app.
	theme.IconNameContentCopy:   "copy.svg",
	theme.IconNameContentCut:    "scissors.svg",
	theme.IconNameContentPaste:  "paste.svg",
	theme.IconNameContentClear:  "eraser.svg",
	theme.IconNameContentAdd:    "plus.svg",
	theme.IconNameContentRemove: "minus.svg",
	theme.IconNameContentUndo:   "arrow-rotate-left.svg",
	theme.IconNameContentRedo:   "arrow-rotate-right.svg",
	theme.IconNameDelete:        "trash-can.svg",
	theme.IconNameConfirm:       "check.svg",
	theme.IconNameCancel:        "xmark.svg",
	theme.IconNameSettings:      "gear.svg",
	theme.IconNameViewRefresh:   "arrows-rotate.svg",
	theme.IconNameAccount:       "user.svg",
	theme.IconNameMenu:          "bars.svg",
	theme.IconNameMenuExpand:    "angle-right.svg",
	theme.IconNameVisibility:    "eye.svg",
	theme.IconNameVisibilityOff: "eye-slash.svg",
	theme.IconNameSearch:        "magnifying-glass.svg",
	theme.IconNameSearchReplace: "magnifying-glass.svg",

	// Documents / mail.
	theme.IconNameDocument:       "file.svg",
	theme.IconNameDocumentCreate: "pen-to-square.svg",
	theme.IconNameDocumentSave:   "floppy-disk.svg",
	theme.IconNameDocumentPrint:  "print.svg",
	theme.IconNameMailForward:    "arrow-up-right-from-square.svg",
	theme.IconNameMailReply:      "reply.svg",
	theme.IconNameMailSend:       "paper-plane.svg",
	theme.IconNameMailAttachment: "paperclip.svg",
	theme.IconNameMailCompose:    "pen-to-square.svg",

	// Navigation / arrows.
	theme.IconNameArrowDropDown:  "caret-down.svg",
	theme.IconNameArrowDropUp:    "caret-up.svg",
	theme.IconNameMoveDown:       "arrow-down.svg",
	theme.IconNameMoveUp:         "arrow-up.svg",
	theme.IconNameNavigateBack:   "arrow-left.svg",
	theme.IconNameNavigateNext:   "arrow-right.svg",
	theme.IconNameMoreHorizontal: "ellipsis.svg",
	theme.IconNameMoreVertical:   "ellipsis-vertical.svg",

	// Dialog status glyphs.
	theme.IconNameInfo:     "circle-info.svg",
	theme.IconNameQuestion: "circle-question.svg",
	theme.IconNameHelp:     "circle-question.svg",
	theme.IconNameWarning:  "triangle-exclamation.svg",
	theme.IconNameError:    "circle-exclamation.svg",

	// File-dialog / storage icons.
	theme.IconNameFolder:     "folder.svg",
	theme.IconNameFolderOpen: "folder-open.svg",
	theme.IconNameFolderNew:  "folder-plus.svg",
	theme.IconNameFile:       "file.svg",
	theme.IconNameFileText:   "file-lines.svg",
	theme.IconNameFileAudio:  "file-audio.svg",
	theme.IconNameFileImage:  "file-image.svg",
	theme.IconNameFileVideo:  "file-video.svg",
	theme.IconNameHome:       "house.svg",
	theme.IconNameComputer:   "computer.svg",
	theme.IconNameDesktop:    "desktop.svg",
	theme.IconNameStorage:    "hard-drive.svg",
	theme.IconNameHistory:    "clock-rotate-left.svg",
	theme.IconNameDownload:   "download.svg",
	theme.IconNameUpload:     "upload.svg",

	// Misc.
	theme.IconNameLogin:          "right-to-bracket.svg",
	theme.IconNameLogout:         "right-from-bracket.svg",
	theme.IconNameList:           "list.svg",
	theme.IconNameGrid:           "table-cells.svg",
	theme.IconNameCalendar:       "calendar-days.svg",
	theme.IconNameViewFullScreen: "expand.svg",
	theme.IconNameViewRestore:    "compress.svg",
	theme.IconNameViewZoomIn:     "magnifying-glass-plus.svg",
	theme.IconNameViewZoomOut:    "magnifying-glass-minus.svg",
	theme.IconNameColorPalette:   "palette.svg",
}

// cowbirdTheme is the default Fyne theme with its icon set swapped for Font
// Awesome. Colours, fonts, and sizes are inherited unchanged.
type cowbirdTheme struct {
	fyne.Theme
	icons map[fyne.ThemeIconName]fyne.Resource
}

// NewCowbirdTheme builds the app theme, pre-loading the Font Awesome resources
// once so Icon lookups (called on every widget render) don't read the embedded
// FS each time. A glyph that fails to load is simply omitted, leaving that name
// to fall back to the default theme.
func NewCowbirdTheme() fyne.Theme {
	icons := make(map[fyne.ThemeIconName]fyne.Resource, len(faIcons))
	for name, file := range faIcons {
		data, err := iconFS.ReadFile(path.Join("icons", file))
		if err != nil {
			continue
		}
		// Wrap as a ThemedResource so Fyne recolours it to the foreground (and
		// re-tints it on high-importance buttons), matching the stock icons.
		icons[name] = theme.NewThemedResource(fyne.NewStaticResource(file, data))
	}
	return &cowbirdTheme{Theme: theme.DefaultTheme(), icons: icons}
}

func (t *cowbirdTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	if res, ok := t.icons[name]; ok {
		return res
	}
	return t.Theme.Icon(name)
}

// faIcon returns a themed Font Awesome glyph by file name (e.g. "lock.svg"), for
// icons that have no Fyne theme-icon name to map through the theme — the toolbar
// lock button, for one. Falls back to the warning glyph if the file is missing.
func faIcon(file string) fyne.Resource {
	data, err := iconFS.ReadFile(path.Join("icons", file))
	if err != nil {
		return theme.Icon(theme.IconNameWarning)
	}
	return theme.NewThemedResource(fyne.NewStaticResource(file, data))
}
