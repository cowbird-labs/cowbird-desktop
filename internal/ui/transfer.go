package ui

import (
	"context"
	"fmt"
	"io"

	"cowbird/internal/core"
	"cowbird/internal/transfer"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// formatNames returns the codec display names in registry order, and a lookup
// from display name back to the codec.
func formatNames() ([]string, map[string]transfer.Codec) {
	codecs := transfer.All()
	names := make([]string, 0, len(codecs))
	byName := make(map[string]transfer.Codec, len(codecs))
	for _, c := range codecs {
		names = append(names, c.Name())
		byName[c.Name()] = c
	}
	return names, byName
}

// showExportItemsDialog lets the user pick a target format, warns that an export
// writes secrets in clear text, then (on confirmation) decrypts the user's owned
// items off the main thread and opens a file-save dialog for the result. The
// warning precedes any work so a user who cancels never produces a plaintext file.
func (m *mainWindow) showExportItemsDialog() {
	var d dialog.Dialog
	names, byName := formatNames()
	formatSel := newEscapableSelect(names, nil)
	formatSel.SetSelected(transfer.Default().Name())
	formatSel.onEscape = func() { d.Hide() }

	form := container.NewVBox(
		widget.NewLabel("Export all of your items to a file."),
		widget.NewForm(widget.NewFormItem("Format", formatSel)),
		widget.NewLabel(
			"The file is saved in PLAIN TEXT and is NOT passphrase-protected —\n"+
				"anyone who can read it can read your passwords. Store it safely or\n"+
				"delete it once imported elsewhere."),
	)

	d = dialog.NewCustomConfirm("Export items", "Export…", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		codec := byName[formatSel.Selected]
		if codec == nil {
			codec = transfer.Default()
		}
		go func() {
			data, err := m.app.ExportItems(context.Background(), codec)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("exporting items: %w", err), m.win)
					return
				}
				m.saveExportFile(data, codec)
			})
		}()
	}, m.win)
	d.Show()
}

// saveExportFile prompts for a location and writes the export bytes there.
// Success is reported only once the file is written and closed without error.
func (m *mainWindow) saveExportFile(data []byte, codec transfer.Codec) {
	save := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if w == nil {
			return // user cancelled
		}
		_, werr := w.Write(data)
		cerr := w.Close()
		if werr != nil {
			dialog.ShowError(fmt.Errorf("writing export file: %w", werr), m.win)
			return
		}
		if cerr != nil {
			dialog.ShowError(fmt.Errorf("closing export file: %w", cerr), m.win)
			return
		}
		dialog.ShowInformation("Items exported",
			"Your items were exported in plain text. Keep the file safe or delete it once imported elsewhere.", m.win)
	}, m.win)
	save.SetFileName("cowbird-export" + codec.Extension())
	save.Show()
}

// showImportItemsDialog lets the user pick the source format, then opens a
// file-picker, reads the chosen file, and imports its items off the main thread.
// The result (imported/skipped counts) is reported and the list reloads so newly
// created items appear.
func (m *mainWindow) showImportItemsDialog() {
	var d dialog.Dialog
	names, byName := formatNames()
	formatSel := newEscapableSelect(names, nil)
	formatSel.SetSelected(transfer.Default().Name())
	formatSel.onEscape = func() { d.Hide() }

	form := container.NewVBox(
		widget.NewLabel("Import items from a file produced by another password manager."),
		widget.NewForm(widget.NewFormItem("Source format", formatSel)),
	)

	d = dialog.NewCustomConfirm("Import items", "Choose file…", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		codec := byName[formatSel.Selected]
		if codec == nil {
			codec = transfer.Default()
		}
		m.openImportFile(codec)
	}, m.win)
	d.Show()
}

func (m *mainWindow) openImportFile(codec transfer.Codec) {
	open := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if r == nil {
			return // user cancelled
		}
		data, rerr := io.ReadAll(r)
		cerr := r.Close()
		if rerr != nil {
			dialog.ShowError(fmt.Errorf("reading import file: %w", rerr), m.win)
			return
		}
		if cerr != nil {
			dialog.ShowError(fmt.Errorf("closing import file: %w", cerr), m.win)
			return
		}

		// A modal progress dialog gives immediate feedback and blocks the menu so
		// the import cannot be started a second time while the first is still
		// writing items (doing so previously created duplicate items).
		progress := dialog.NewCustomWithoutButtons("Importing…", widget.NewProgressBarInfinite(), m.win)
		progress.Show()

		go func() {
			res, err := m.app.ImportItems(context.Background(), codec, data)
			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					dialog.ShowError(fmt.Errorf("importing %s file: %w", codec.Name(), err), m.win)
					return
				}
				m.reportImport(res)
				m.reload()
			})
		}()
	}, m.win)
	open.Show()
}

// showRemoveDuplicatesDialog scans for exact-duplicate owned items and, after
// confirming the count with the user, removes the extra copies (keeping one of
// each). It is the cleanup path for an accidental double-import. The scan and
// the removal both run off the main thread behind a modal progress dialog.
func (m *mainWindow) showRemoveDuplicatesDialog() {
	progress := dialog.NewCustomWithoutButtons("Scanning for duplicates…", widget.NewProgressBarInfinite(), m.win)
	progress.Show()

	go func() {
		count, err := m.app.RemoveDuplicateItems(context.Background(), true) // dry run
		fyne.Do(func() {
			progress.Hide()
			if err != nil {
				dialog.ShowError(fmt.Errorf("scanning for duplicates: %w", err), m.win)
				return
			}
			if count == 0 {
				dialog.ShowInformation("No duplicates", "No exact-duplicate items were found.", m.win)
				return
			}
			msg := fmt.Sprintf("Found %d exact-duplicate item(s). Remove the extra copies?\n"+
				"One copy of each is kept. This cannot be undone.", count)
			dialog.ShowConfirm("Remove duplicates", msg, func(ok bool) {
				if !ok {
					return
				}
				m.removeDuplicates()
			}, m.win)
		})
	}()
}

func (m *mainWindow) removeDuplicates() {
	progress := dialog.NewCustomWithoutButtons("Removing duplicates…", widget.NewProgressBarInfinite(), m.win)
	progress.Show()

	go func() {
		removed, err := m.app.RemoveDuplicateItems(context.Background(), false)
		fyne.Do(func() {
			progress.Hide()
			if err != nil {
				// Some copies may have been removed before the error; reload to
				// reflect the partial result.
				dialog.ShowError(fmt.Errorf("removing duplicates: %w", err), m.win)
				m.reload()
				return
			}
			dialog.ShowInformation("Duplicates removed", fmt.Sprintf("Removed %d duplicate item(s).", removed), m.win)
			m.reload()
		})
	}()
}

// reportImport shows the import outcome, mentioning skipped entries only when
// there were any.
func (m *mainWindow) reportImport(res core.ImportResult) {
	msg := fmt.Sprintf("Imported %d item(s).", res.Imported)
	if res.Skipped > 0 {
		msg += fmt.Sprintf("\nSkipped %d entry(ies) that could not be read.", res.Skipped)
	}
	dialog.ShowInformation("Import complete", msg, m.win)
}
