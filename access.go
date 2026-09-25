//go:build windows

package main

// Somebody else's service is on the port: whether this window can get in is then something
// only the window knows.
//
// Go's probe carries no cookies, so such a service answers it 401 for ever, while the
// window's cookie store is persistent and may still hold a session from an earlier run. So
// the window is asked to fetch its own address once, with same-origin credentials, and only
// a real refusal (401/403) becomes the notice page.

import (
	_ "embed"
	"log/slog"
	"time"
)

// accessAnswer is where the window answers. Buffered by one: an answer can arrive a little
// after the moment it is waited for, and should not be blocked by that.
var accessAnswer = make(chan int, 1)

// accessWait is how long an answer is waited for. Waiting in vain does nothing at all: the page
// is already showing that address, and calling it broken because it did not answer is worse
// than leaving it there.
const accessWait = 5 * time.Second

//go:embed js/askWindow.js
var askWindowJS string

// verifyWindowAccess asks the window once the address is open, and replaces it with the notice
// page only when it was really refused.
func verifyWindowAccess() {
	go func() {
		// 等页面落地：Eval 要有文档之后才有意义。
		time.Sleep(time.Second)
		win.Dispatch(win.w.Eval, askWindowJS)
		select {
		case status := <-accessAnswer:
			if status == 401 || status == 403 {
				slog.Warn("the window was refused, showing the notice", "status", status)
				showNotice(blockedPage())
				return
			}
			slog.Info("the window can reach the service", "address", serviceAddress, "status", status)
		case <-time.After(accessWait):
			slog.Warn("the window did not answer, leaving the page as it is", "waited", accessWait)
		}
	}()
}
