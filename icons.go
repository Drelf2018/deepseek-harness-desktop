//go:build windows

package main

// The window's own icons: the artwork cut to the two sizes Windows asks a window for, handed
// over with WM_SETICON. This is not the icon inside the .exe - that one is a resource, written
// by internal/genicon. A window's icon is a runtime thing, and the class go-webview2 registers
// has none: without this, the taskbar, Alt+Tab and the title bar fall back to the default.

import (
	"fmt"
	"log/slog"
	"unsafe"

	artwork "github.com/Drelf2018/deepseek-harness-desktop/internal/artwork"
)

// setWindowIcons gives the window the icons the taskbar, Alt+Tab and the title bar draw.
//
// Each one is drawn at the size Windows says it wants rather than at 16 and 32: a scaled
// display asks for larger icons (SM_CXSMICON is 24 at 150%, SM_CXICON 48) and does not
// scale up what it is handed, so an icon drawn at 16 and shown at 24 is a stretched one,
// which is the blur. Drawing the artwork again at the asked-for size costs almost nothing
// and comes out sharp.
//
// The HICONs are deliberately not destroyed afterwards: they belong to the window and have
// to outlive this call, and the end of the process is what releases them.
func (win *webviewWindow) setWindowIcons() error {
	icons := make([]uintptr, 0, 2)
	sizes := make([]int, 0, 2)
	for _, want := range []struct {
		metric   uintptr
		fallback int
		slot     uintptr
		where    string
	}{
		{smCXSmallIcon, 16, iconSlotSmall, "title bar"},
		{smCXIcon, 32, iconSlotBig, "taskbar and Alt+Tab"},
	} {
		size := want.fallback
		if got, _, _ := procGetSystemMetrics.Call(want.metric); got > 0 {
			size = int(got)
		}
		sizes = append(sizes, size)

		blob, err := artwork.IconICO(size)
		if err != nil {
			return fmt.Errorf("%dpx icon for the %s: %w", size, want.where, err)
		}
		_, image, err := artwork.IconEntry(blob, size)
		if err != nil {
			return fmt.Errorf("%dpx icon for the %s: %w", size, want.where, err)
		}
		// fIcon 是 1：这是图标，不是光标。
		hicon, _, err := procCreateIconFromResourceEx.Call(
			uintptr(unsafe.Pointer(&image[0])), uintptr(len(image)),
			1, iconResourceVersion, uintptr(size), uintptr(size), 0)
		if hicon == 0 {
			return fmt.Errorf("%dpx icon for the %s: %v", size, want.where, err)
		}
		icons = append(icons, hicon)
		procSendMessageW.Call(win.hwnd, wmSetIcon, want.slot, hicon)
	}

	// 类上也挂同样两张图，给那种去类里找图标的系统用。
	// 用这个类的只有本程序的窗口，所以不影响别的东西。
	procSetClassLongPtrW.Call(win.hwnd, gclpHIconSmall, icons[0])
	procSetClassLongPtrW.Call(win.hwnd, gclpHIcon, icons[1])

	slog.Info("window icons", "titleBar", sizes[0], "taskbar", sizes[1])
	return nil
}

// smallIconSize is the edge the notification area draws at: SM_CXSMICON, which follows the
// display scaling (16 at 100%, 20 at 125%, 24 at 150%).
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
