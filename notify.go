//go:build windows

package main

// Windows notifications for the three panels that wait on a person: an approval, a question, a
// plan under review. The window shows the panel, but the window may be behind everything else
// - and these are exactly the moments the app is waiting for someone.
//
// The page is the only side that can see a panel, so the page decides when to ask
// (js/notify.js); this file is the other half: a binding to receive that call, and the toast
// that shows it (notify.ps1).

import (
	_ "embed"
	"encoding/base64"
	"encoding/xml"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode/utf16"

	"github.com/Drelf2018/deepseek-harness-desktop/internal/artwork"
)

// notifyBinding is the name the page calls. It is written in two places - the binding below and
// the call in js/notify.js - and the two drifting apart is silent: no toast, no error, nothing
// in the log. notify_test.go is what keeps them together.
const notifyBinding = "_notify"

// The page's half. The app installs the script into every document it is about to create, so
// it runs before the page's own code and again after a reload.
//
//go:embed js/notify.js
var notifyJS string

// watchForPrompts installs the page's half. It must happen before Run: the script is
// registered for the documents about to be created, and the document already open is not one
// of them.
func watchForPrompts() {
	win.w.Init(notifyJS)
}

// notificationScheme is the URL scheme a click on a notification comes back through,
// notificationURL is the address itself.
//
// It is this app's own scheme rather than an http one because that is how Windows hands a
// launch back: the shell starts the program registered for the scheme, that copy finds the
// single-instance mutex taken, and asks the copy already running to come forward
// (instance.go). Nothing parses the address - it only has to be one only this app answers to.
const (
	notificationScheme = appID
	notificationURL    = notificationScheme + "://show"
)

// registryClasses is where Windows keeps the per-user registrations this file writes: file
// types, URL protocols, and the AppUserModelIDs they are named by. One constant because every
// write goes under it, and a typo in it is not an error - reg.exe fails, runHidden logs a
// warning, and what the user sees is a notification that never appears, or a click on one that
// does nothing.
const registryClasses = `HKCU\Software\Classes\`

// prepareNotifications registers the three things the system has to know before a
// notification can appear: the name it is shown under, the picture beside that name, and the
// scheme a click comes back through.
//
// DisplayName is what the toast credits. Without it the notification is attributed to Windows
// PowerShell, which is the process that actually shows it. IconUri is a file path, so the icon
// has to exist as a file first - notificationIcon writes it (the drawing itself is embedded in
// the .exe, and nothing beside an .exe can be relied on) - and the same picture travels inside
// every toast as well, so a notification shows it even when the registered copy is not what the
// shell is reading.
//
// Both are written every run rather than once: the scheme names the .exe by its current path,
// and an install that has been moved would otherwise keep calling the copy that used to be
// there.
//
// Called on a goroutine - nothing waits for it, since the first panel cannot appear until the
// service is up seconds later - because four reg.exe runs are not worth a slower startup.
func prepareNotifications() {
	runHidden("reg", "add", registryClasses+`AppUserModelId\`+appID,
		"/v", "DisplayName", "/t", "REG_SZ", "/d", appName, "/f")
	// 图标失败不算失败：署名还在，通知照弹，只是没有那幅画。
	if path := notificationIcon(); path != "" {
		runHidden("reg", "add", registryClasses+`AppUserModelId\`+appID,
			"/v", "IconUri", "/t", "REG_SZ", "/d", path, "/f")
	}

	exe, err := os.Executable()
	if err != nil {
		slog.Warn("notification: cannot register the scheme", "error", err)
		return
	}
	runHidden("reg", "add", registryClasses+notificationScheme,
		"/ve", "/t", "REG_SZ", "/d", "URL:"+appName, "/f")
	runHidden("reg", "add", registryClasses+notificationScheme,
		"/v", "URL Protocol", "/t", "REG_SZ", "/d", "", "/f")
	// 没有 shell\open\command 时，协议注册着，点了却什么都不会发生：命令行里那个 %1 就是
	// 通知带上来的地址，本程序不解析它，只是被它叫起来。
	runHidden("reg", "add", registryClasses+notificationScheme+`\shell\open\command`,
		"/ve", "/t", "REG_SZ", "/d", `"`+exe+`" "%1"`, "/f")
}

// smallIconSize is the edge the notification area draws at: SM_CXSMICON, which follows the
// display scaling (16 at 100%, 20 at 125%, 24 at 150%).
//
// Three things come out of it: the tray icon, the window's own icons, and the picture beside a
// notification's title - the last two are drawn at the size they will be drawn, because Windows
// does not scale up what it is handed.
//
// It lives here rather than in internal/artwork because it is the one part of the drawing that
// asks Windows something, and that package is deliberately platform-independent - it has to be,
// because the release workflow runs its generator on Linux.
func smallIconSize() int {
	if got, _, _ := procGetSystemMetrics.Call(smCXSmallIcon); got > 0 {
		return int(got)
	}
	return 16
}

// notificationIcon writes those bytes where the shell can read them and returns the path; "" means
// it could not be written, and the registration goes ahead without it.
//
// A file because IconUri takes one: the shell reads it, it cannot be handed bytes. Written on every
// run rather than kept, because the drawing lives in the .exe and the next build may draw a
// different mark.
func notificationIcon() string {
	blob, err := artwork.IconICO(smallIconSize())
	if err != nil {
		slog.Warn("notification: cannot draw the icon", "error", err)
		return ""
	}
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		slog.Warn("notification: cannot create the data directory", "path", appDataDir(), "error", err)
		return ""
	}
	path := filepath.Join(appDataDir(), "notification-icon.ico")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		slog.Warn("notification: cannot write the icon", "path", path, "error", err)
		return ""
	}
	return path
}

// runHidden runs a console program without a window, and reports only whether it failed.
func runHidden(name string, args ...string) {
	cmd := hiddenCommand(name, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		slog.Warn("a helper command failed", "path", name, "error", err)
	}
}

// notifyPS is notify.ps1: the PowerShell that shows one notification, payload and all.
//
//go:embed notify.ps1
var notifyPS string

// notifyTemplate is notifyPS parsed once, with the function it is given: xml, which escapes
// the page's own text for the payload.
//
// Must: the file is in this repository, and a template that does not parse is a typo that shows up
// the moment the program is run - or the moment the tests are, because the panic is in the package
// initialiser and takes the whole test binary with it. There is nothing for a caller to do with the
// error, and a -H=windowsgui build has no stderr to print it to either.
var notifyTemplate = template.Must(template.New("notify.ps1").Funcs(template.FuncMap{"xml": escapeXML}).Parse(notifyPS))

// notifyView is what notify.ps1 is filled with.
//
// Launch and Notifier are the app's own identity - the address a click comes back through, and
// the AppUserModelID the notification is shown under - and they are the reason the script is a
// template: both are derived from appID, which is written down once and must never be repeated
// (see the const block in main.go).
type notifyView struct {
	Launch   string
	Notifier string
	Title    string
	Body     string
}

// renderNotify fills notify.ps1 in. It is what turns one panel into one command line.
func renderNotify(title, body string) (string, error) {
	var b strings.Builder
	err := notifyTemplate.Execute(&b, notifyView{
		Launch:   notificationURL,
		Notifier: appID,
		Title:    title,
		Body:     body,
	})
	return b.String(), err
}

// installNotifications wires the page's half to the app's: the binding it calls, and the script
// that calls it. Both must be in place before Run - they are registered for the documents about
// to be created, and the document already open is not one of them.
//
// The page checks for the binding before calling it, so a bind that failed costs the notifications
// and nothing else - worth a line in the log, not a stopped program.
func installNotifications() {
	if err := win.Bind(notifyBinding, func(title, body string) { go fireToast(title, body) }); err != nil {
		slog.Warn("bind _notify failed", "error", err)
		return
	}
	watchForPrompts()
}

// fireToast shows one notification.
//
// Windows' toast API is WinRT, which a Go program cannot reach without a COM pile this app has
// no other use for. Windows PowerShell can, and 5.1 is on every Windows 10 and 11, so the
// payload is handed to it. The caller runs this on a goroutine: the binding it comes from runs
// on the window's thread, and starting a process takes long enough that the window would
// visibly pause.
func fireToast(title, body string) {
	slog.Info("showing a notification", "title", title)

	script, err := renderNotify(title, body)
	if err != nil {
		slog.Warn("notification: cannot build the toast script", "error", err)
		return
	}
	cmd := hiddenCommand("powershell.exe",
		"-NoProfile", "-WindowStyle", "Hidden", "-EncodedCommand", utf16leBase64(script))
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		slog.Warn("notification: the toast could not be shown", "error", err)
	}
}

// escapeXML is the template's xml function: it escapes one run of text for the payload, which
// is XML. The apostrophe matters as much as the ampersand - the payload sits inside a
// single-quoted PowerShell string, and one left in it would end that string and cut
// notify.ps1 in half.
//
// func(string) (string, error) because that is what text/template takes. The error is
// xml.EscapeText's, and a strings.Builder is the one writer that cannot produce one.
func escapeXML(s string) (string, error) {
	var b strings.Builder
	err := xml.EscapeText(&b, []byte(s))
	return b.String(), err
}

// utf16leBase64 encodes a script the way -EncodedCommand wants it: UTF-16LE, then base64.
//
// Not a command line, for two reasons. The script has quotes and newlines of its own, which a
// command line would make the shell's problem. And the payload may be Chinese: a command line
// reaches PowerShell through the process's ANSI code page, where those characters are not the
// same characters any more.
func utf16leBase64(s string) string {
	units := utf16.Encode([]rune(s))
	data := make([]byte, len(units)*2)
	for i, u := range units {
		data[i*2] = byte(u)
		data[i*2+1] = byte(u >> 8)
	}
	return base64.StdEncoding.EncodeToString(data)
}
