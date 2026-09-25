//go:build windows

package main

// The window's own thread: the procedure that receives its messages, and the queue of work
// handed to it from the goroutines that must not touch WebView2 themselves.

import (
	_ "embed"
	"log/slog"
	"sync"
	"time"
)

// wmRunOnWindowThread is the message Dispatch posts to the window. It is WM_APP, the value
// go-webview2 wakes its own dispatch queue with: a window message carrying it does both jobs -
// it runs what this app queued, and it lets the library flush what was queued through it.
const wmRunOnWindowThread = 0x8000

// wndProc hides the window instead of closing it, so the tray survives a click on
// the X, and it runs what Dispatch queued - this procedure is the window's thread, and it is
// reached by whichever loop is pumping. Every other message goes to the procedure go-webview2
// installed - including WM_GETMINMAXINFO, which is how the minimum size below takes effect.
func (win *webviewWindow) wndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	if msg == wmRunOnWindowThread {
		runOnWindowThread()
		return 0
	}
	if msg == wmClose {
		procShowWindow.Call(hwnd, swHide)
		return 0
	}
	if msg == wmSysCommand {
		// 低四位由系统占用，比对自己那几个 id 之前先抹掉。
		if handleSystemCommand(wparam &^ 0xF) {
			return 0
		}
	}
	res, _, _ := procCallWindowProcW.Call(win.previous, hwnd, msg, wparam, lparam)
	return res
}

func (win *webviewWindow) Navigate(url string) { win.w.Navigate(url) }

// Reload asks the page to load itself again, at the address it is on right now.
//
// That address is the page's own, not the one the app would navigate to. After the service
// announces its link the page carries a one-time token, and rebuilding the address from the
// saved state would replace a page that works with one that is refused; a reload keeps what
// is there and asks for it again, which is what a person picking 刷新 means.
//
//go:embed js/reload.js
var reloadJS string

func (win *webviewWindow) Reload() {
	win.Dispatch(win.w.Eval, reloadJS)
}

// The work waiting for the window's thread, and the lock that guards it.
var (
	uiMu    sync.Mutex
	uiQueue []uiWork
)

// uiWork is one queued call and the moment it was queued, so that a call that ran late can say
// how late it was.
type uiWork struct {
	at time.Time
	f  func()
}

// Dispatch runs f on the thread that owns the window: the one the message loop and the
// WebView2 controller both live on. The menu's goroutines are not that thread, and a
// controller call made from one of them does nothing at all.
//
// It posts a message to the *window*, not to the thread (which is what the library's
// Dispatch does). A thread message belongs to no window, so a modal loop - a menu, a message
// box - retrieves it and DispatchMessageW has nothing to deliver it to; everything queued
// behind it then waits for the next thing to post one. The rule is this one line.
//
// The call and its argument are passed apart rather than closed over, so that the call which
// will run is written at the call site instead of living inside a closure body - and so that
// the argument goes through the compiler's type check against the function it is for. A body
// that is not one call goes in as a closure taking an unused argument, as window_test.go does.
func (win *webviewWindow) Dispatch[T any](f func(T), t T) {
	uiMu.Lock()
	uiQueue = append(uiQueue, uiWork{at: time.Now(), f: func() { f(t) }})
	uiMu.Unlock()
	procPostMessageW.Call(win.hwnd, wmRunOnWindowThread, 0, 0)
}

// runOnWindowThread runs what Dispatch queued. It is called from the window procedure, which is
// the window's thread by definition - the library's loop or a menu's modal loop, whichever is
// pumping at that moment.
func runOnWindowThread() {
	uiMu.Lock()
	queued := uiQueue
	uiQueue = nil
	uiMu.Unlock()
	for _, work := range queued {
		if late := time.Since(work.at); late > time.Second {
			// 排进来和跑起来隔了这么久：有人把唤醒消息吃掉了。这一条是留给下一次的线索。
			slog.Warn("queued work ran late", "late", late.Round(time.Millisecond))
		}
		work.f()
	}
}

// Bind publishes a Go function to the page as window.<name>. It has to happen before Run:
// it registers a script for every document about to be created, and the document already
// open is not one of them.
func (win *webviewWindow) Bind(name string, fn any) error { return win.w.Bind(name, fn) }
