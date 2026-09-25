//go:build windows

package main

// Where this app's own files are: the data folder under %LOCALAPPDATA%, the log inside it, the
// window state beside it, and the folder the running .exe sits in.

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// windowState is what the app remembers between runs: the size the window was left at,
// whether it was maximised, and the address it was showing. It is window-state.json.
//
// The size is a *window rectangle*, borders included, because that is what CreateWindowExW
// is given when the window is built. There is no position here on purpose: within a run
// Windows keeps a hidden window's rectangle, and across runs the window opens centred.
// URL empty means the file has nothing to say and the caller uses its own default.
type windowState struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Maximized bool   `json:"maximized"`
	URL       string `json:"url"`

	// Size is what the three menus were set to. The window rectangle above is the size the
	// window was actually left at - which a drag can move away from what the menus describe -
	// so this is kept apart from it: the rectangle says how big to open, this says which
	// entry to tick when the menus are built.
	Size sizeOnDisk `json:"size"`

	// OnTop is whether the window was left above the others. It is put back on the next run,
	// and the tick beside 置顶 comes from the window itself, so this is read from there on the
	// way out rather than kept in a variable of its own.
	OnTop bool `json:"onTop"`
}

// LogValue renders the state as a group, so a "state" field in the log keeps the field names
// the old %+v showed, and stays readable to anything that reads the log back.
func (s windowState) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("width", s.Width),
		slog.Int("height", s.Height),
		slog.Bool("maximized", s.Maximized),
		slog.String("url", s.URL),
		slog.Bool("onTop", s.OnTop),
	)
}

// appDataDir is the folder this app owns: everything it writes belongs under it (the
// WebView2 profile in EBWebView inside it is the one thing it does not own). Left to
// themselves go-webview2 would name a folder after the .exe and WebView2 would put its
// profile beside it, so the app names its own.
func appDataDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), appID)
}

// logPath is the file the app writes its own diagnostics to.
func logPath() string {
	return filepath.Join(appDataDir(), "app.log")
}

// programDir is the folder the running .exe sits in, which the menu opens as 程序目录. It
// belongs to whoever put the folder there, unlike appDataDir - the only place this program
// writes.
func programDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// openLog opens the log file, starting a fresh one once the last has grown large.
func openLog() (*os.File, error) {
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		return nil, err
	}
	path := logPath()
	// 程序运行期间没有别的东西会轮转这个文件，用上几个月的安装会把它撑得没有尽头：
	// 把上一份留下，重新开一个。
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		old := path + ".old"
		// 文件还在的时候，Windows 不允许改名覆盖它。
		_ = os.Remove(old)
		if err := os.Rename(path, old); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

// setupLogging sends the app's log records to a file under appDataDir.
//
// There is no console (-H=windowsgui), so without this every line would go to a stderr
// nobody can see. One slog.SetDefault covers the standard log package and the library,
// which logs through slog.Default; failing to open the file is not fatal.
func setupLogging() {
	w, err := openLog()
	if err != nil {
		slog.Warn("logging to a file", "error", err)
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
}

// windowStatePath is the file the window's size and maximised state are remembered in.
func windowStatePath() string {
	return filepath.Join(appDataDir(), "window-state.json")
}

// loadWindowState is the state the last run recorded.
//
// A missing file, an unreadable one, and one whose size cannot describe a window all
// mean the same thing to the caller: there is nothing to restore, so the window opens
// the way the menu settings ask for.
func loadWindowState() (windowState, bool) {
	data, err := os.ReadFile(windowStatePath())
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("window state: cannot read it", "path", windowStatePath(), "error", err)
		}
		return windowState{}, false
	}
	var s windowState
	if err := json.Unmarshal(data, &s); err != nil {
		slog.Warn("window state: cannot parse it", "path", windowStatePath(), "error", err)
		return windowState{}, false
	}
	if s.Width <= 0 || s.Height <= 0 {
		slog.Warn("window state: not a window size, ignoring it", "path", windowStatePath(), "state", s)
		return windowState{}, false
	}
	// 旧版本可能把带一次性 token 的地址原样存了下来；那种 token 这一趟用不了，先摘掉。
	s.URL = stripToken(s.URL)
	return s, true
}

// saveWindowState records the size the window was left at, for the next run.
func saveWindowState(s windowState) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		slog.Error("window state: cannot encode it", "error", err)
		return
	}
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		slog.Error("window state: cannot create the data directory", "path", appDataDir(), "error", err)
		return
	}
	if err := os.WriteFile(windowStatePath(), append(data, '\n'), 0o644); err != nil {
		slog.Error("window state: cannot write it", "path", windowStatePath(), "error", err)
		return
	}
	slog.Info("window state saved", "path", windowStatePath(), "state", s)
}
