package ui

import (
	"bytes"
	"path"
	"strings"
	"testing"

	"fyne.io/fyne/v2/theme"
)

// TestFontAwesomeIconsEmbedded checks every glyph named in faIcons is actually
// present in the embedded FS. A missing file would silently fall back to the
// stock icon, so this guards against a typo in the mapping.
func TestFontAwesomeIconsEmbedded(t *testing.T) {
	for name, file := range faIcons {
		data, err := iconFS.ReadFile(path.Join("icons", file))
		if err != nil {
			t.Errorf("icon %q -> %q: not embedded: %v", name, file, err)
			continue
		}
		if !bytes.Contains(data, []byte("<path")) {
			t.Errorf("icon %q -> %q: no <path> element, will not render", name, file)
		}
	}
}

// TestCowbirdThemeIconOverride confirms a mapped name returns the Font Awesome
// glyph while an unmapped name falls back to the default theme. Resource names
// are compared rather than Content() so the test needs no running Fyne app
// (ThemedResource.Content colorizes via theme.Color, which requires one).
func TestCowbirdThemeIconOverride(t *testing.T) {
	th := NewCowbirdTheme()
	def := theme.DefaultTheme()

	// ThemedResource.Name() is "<color>_<sourceName>", e.g. "foreground_copy.svg".
	if got := th.Icon(theme.IconNameContentCopy).Name(); !strings.HasSuffix(got, "copy.svg") {
		t.Errorf("ContentCopy not overridden by Font Awesome: got name %q", got)
	}

	// IconNameCheckButton is intentionally left to the default theme.
	if th.Icon(theme.IconNameCheckButton).Name() != def.Icon(theme.IconNameCheckButton).Name() {
		t.Error("unmapped icon should fall back to the default theme")
	}
}
