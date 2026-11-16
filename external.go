//go:build windows

package main

// Links that leave the app. Some the page opens itself, through window.open; others - a bare
// anchor, an address that a component turned into a link but did not take over - it leaves to the
// engine. Either way WebView2 has no window of its own to put them in, so left alone it grows a
// chromeless popup inside this app. Handing the address to the shell instead is what "open that in
// my browser" means: the default browser, with the profile and the cookies the user actually
// browses with.
//
// The page decides when to ask (js/openExternal.js); this file decides what may be opened.

import (
	_ "embed"
	"log/slog"
	neturl "net/url"
)

// openExternalBinding is the name the page calls. Like notifyBinding it is written in two places -
// here and in js/openExternal.js - and external_test.go is what keeps the two together.
const openExternalBinding = "_openExternal"

// The page's half. Installed into every document the window is about to create, so it runs before
// the page's own code and again after a reload.
//
//go:embed js/openExternal.js
var openExternalJS string

// installExternalLinks wires the page's half to the app's. Both must be in place before Run: the
// script is registered for the documents about to be created, and the document already open is not
// one of them.
func installExternalLinks() {
	if err := win.Bind(openExternalBinding, openExternal); err != nil {
		slog.Warn("bind _openExternal failed", "error", err)
		return
	}
	win.w.Init(openExternalJS)
}

// openExternal hands an address to the shell, if it is one a page is allowed to launch: http, https
// or mailto. Anything else is refused rather than experimented with - the page is the side that
// chose the address, and file: in particular would open things this app has no business opening.
func openExternal(raw string) {
	target, ok := externalAddress(raw)
	if !ok {
		slog.Warn("external link refused", "url", raw)
		return
	}
	slog.Info("opening a link in the default browser", "url", target)
	shellOpen(target)
}

// externalAddress answers with the address to hand over, if the shell may open it.
func externalAddress(raw string) (string, bool) {
	u, err := neturl.Parse(raw)
	if err != nil {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		// An address with no host is not something to hand to ShellExecute.
		if u.Host == "" {
			return "", false
		}
	case "mailto":
		// No host to check: what follows the colon is the whole address.
	default:
		return "", false
	}
	return u.String(), true
}
