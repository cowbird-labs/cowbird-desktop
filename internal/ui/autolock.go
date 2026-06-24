package ui

import (
	"time"

	"cowbird/internal/config"
	"cowbird/internal/core"

	"fyne.io/fyne/v2"
)

// statusFlashDuration is how long a transient status message (e.g. a copy
// confirmation) stays before the status bar collapses again.
const statusFlashDuration = 4 * time.Second

// autoLockDuration converts the persisted auto-lock preference into a timer
// duration. A zero result means auto-lock is disabled (the toggle is off or the
// configured value is non-positive).
func autoLockDuration(ui config.UI) time.Duration {
	if !ui.AutoLock || ui.AutoLockMinutes <= 0 {
		return 0
	}
	return time.Duration(ui.AutoLockMinutes) * time.Minute
}

// clipboardClearDuration converts the persisted clipboard-clearing preference
// into a delay. A zero result means the feature is disabled.
func clipboardClearDuration(ui config.UI) time.Duration {
	if !ui.ClipboardClear || ui.ClipboardClearSeconds <= 0 {
		return 0
	}
	return time.Duration(ui.ClipboardClearSeconds) * time.Second
}

// startAutoLock (re)arms the inactivity timer from m.autoLockDur. A zero
// duration disables auto-lock. Safe to call repeatedly — e.g. after the timeout
// is changed in settings — and is main-thread only (m.autoLockTimer is only
// touched from the Fyne thread).
func (m *mainWindow) startAutoLock() {
	if m.autoLockTimer != nil {
		m.autoLockTimer.Stop()
		m.autoLockTimer = nil
	}
	if m.autoLockDur <= 0 {
		return
	}
	// AfterFunc fires on its own goroutine, so the lock work returns to the Fyne
	// thread via fyne.Do.
	m.autoLockTimer = time.AfterFunc(m.autoLockDur, func() {
		fyne.Do(m.lock)
	})
}

// stopAutoLock disarms the inactivity timer.
func (m *mainWindow) stopAutoLock() {
	if m.autoLockTimer != nil {
		m.autoLockTimer.Stop()
		m.autoLockTimer = nil
	}
}

// noteActivity resets the inactivity countdown. It is called from the
// interaction points the window can observe (list selection, search/filter
// changes, copies, menu and dialog launches, and stray key events). Fyne offers
// no global input hook, so coverage is interaction-based rather than raw input:
// a window left untouched — even if the pointer hovers over it — eventually
// locks, which is the desired behaviour for a password manager. No-op when
// auto-lock is disabled.
func (m *mainWindow) noteActivity() {
	if m.autoLockTimer != nil && m.autoLockDur > 0 {
		m.autoLockTimer.Reset(m.autoLockDur)
	}
}

// lock tears down the decrypted session and returns to the unlock screen,
// reusing the existing Vault connection so only the unlock password is needed
// to resume. The in-memory identity is dropped with the old main window. This
// mirrors the reconnect path in editConnection, minus the Vault swap.
func (m *mainWindow) lock() {
	a := fyne.CurrentApp()
	m.stopAutoLock()

	// Don't leave a secret Cowbird put on the clipboard sitting there after a
	// lock. Only clear when the clipboard still holds the last value we copied,
	// so we never wipe something the user copied from elsewhere.
	if m.lastClipboardValue != "" && m.win.Clipboard().Content() == m.lastClipboardValue {
		m.win.Clipboard().SetContent("")
	}

	v := m.app.Vault
	unlockW := NewUnlockWindow(a, v, func(coreApp *core.App) {
		NewMainWindow(a, coreApp, m.tray).Show()
	})
	m.tray.Attach(unlockW) // keep close-to-tray during the re-unlock
	unlockW.Show()
	m.win.Close()
}

// copyToClipboard copies value to the clipboard, updates the status bar, and —
// when clipboard clearing is enabled — schedules the value to be wiped after the
// configured delay. The scheduled clear only fires if the clipboard still holds
// the same value, so a later copy (here or in another app) is never clobbered.
func (m *mainWindow) copyToClipboard(value, status string) {
	m.win.Clipboard().SetContent(value)
	if status != "" {
		m.setStatus(status)
		// The copy confirmation is transient feedback; clear it after a moment so
		// the status bar collapses again rather than lingering. Only clear if it
		// is still our message (a later status update takes precedence).
		time.AfterFunc(statusFlashDuration, func() {
			fyne.Do(func() {
				if m.status.Text == status {
					m.setStatus("")
				}
			})
		})
	}
	m.lastClipboardValue = value
	m.noteActivity()

	if m.clipClearDur <= 0 {
		return
	}
	time.AfterFunc(m.clipClearDur, func() {
		fyne.Do(func() {
			if m.win.Clipboard().Content() != value {
				return
			}
			m.win.Clipboard().SetContent("")
			if m.lastClipboardValue == value {
				m.lastClipboardValue = ""
			}
		})
	})
}
