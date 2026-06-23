package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// Tray owns the system-tray icon and the close-to-tray behaviour.
//
// It MUST be installed before the Fyne run loop starts: the glfw driver invokes
// the tray's start hook exactly once, at the top of its run loop, and only if a
// tray menu was registered beforehand. Registering a tray menu later (e.g. once
// the main window opens) silently fails to launch the tray loop — the icon
// never appears even though a hidden monitor window keeps the process alive.
// So NewTray is called from main before the first window's ShowAndRun, and the
// resulting Tray is pointed at each primary window as the startup flow advances.
type Tray struct {
	app fyne.App
	win fyne.Window // the window the tray's "Show" action raises
}

// NewTray registers the system-tray menu and returns a controller for it. It
// returns nil on drivers without a system tray (mobile/wasm), so callers can
// hold a possibly-nil *Tray and rely on its nil-safe methods. Call this before
// app.Run()/ShowAndRun().
//
// icon, when non-nil, is used for the tray specifically (distinct from the app
// window icon); it must be set before the run loop starts so the tray's ready
// hook picks it up rather than falling back to the app icon.
func NewTray(a fyne.App, icon fyne.Resource) *Tray {
	desk, ok := a.(desktop.App)
	if !ok {
		return nil
	}
	t := &Tray{app: a}
	// Fyne appends a "Quit" item automatically, so only the show action is set
	// here. The label/order of the appended Quit is handled by the driver.
	menu := fyne.NewMenu("Cowbird",
		fyne.NewMenuItem("Show Cowbird", t.show),
	)
	desk.SetSystemTrayMenu(menu)
	if icon != nil {
		desk.SetSystemTrayIcon(icon)
	}
	return t
}

// Attach points the tray at w and makes the user closing w hide it (keeping
// Cowbird running in the tray) rather than quit. Nil-safe: with no tray (the
// preference is off or the driver has none) the window keeps Fyne's default
// close behaviour, which quits when the last window closes.
func (t *Tray) Attach(w fyne.Window) {
	if t == nil {
		return
	}
	t.win = w
	w.SetCloseIntercept(func() { w.Hide() })
}

// show raises the most recently attached window. Programmatic Window.Close in
// the startup flow swaps t.win to the replacement before the old one closes, so
// this always targets a live window.
func (t *Tray) show() {
	if t == nil || t.win == nil {
		return
	}
	t.win.Show()
	t.win.RequestFocus()
}
