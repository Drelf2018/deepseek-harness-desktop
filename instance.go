//go:build windows

package main

// One copy of the app, however many times it is started: a second start finds the copy that
// is already up and brings its window forward, instead of leaving a second tray, window and
// service behind. A named mutex says who is first, a named event asks it to come forward.

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// instanceMutex names the lock that decides who is first, and instanceEvent names the event
// a second start sets. Both are built from appID, which must never change: a rename would
// let a new build start beside the old one.
const (
	instanceMutex = appID + "_SingleInstance"
	instanceEvent = appID + "_Show"

	// restartFlag is the argument 重启 gives the copy it starts, and restarting says this copy
	// is the one that carries it. It means one thing: the copy that started this one is on its
	// way out, so keep looking for the mutex instead of concluding that a copy is already up.
	restartFlag = "-restart"

	// How long a copy started by 重启 keeps looking for the mutex, and how often it looks. The
	// predecessor exits quickly - it writes its state, stops its service, destroys its window -
	// so this is a ceiling for a stuck one, not a budget anybody expects to spend.
	restartWait = 15 * time.Second
	restartPoll = 50 * time.Millisecond
)

// restarting says this copy was started by 重启, so it has a predecessor to wait for.
var restarting bool

// The two handles the first copy holds for the life of the process, and deliberately never
// closes. Letting go of the mutex would let the next copy start beside this one; letting go
// of the event would take the object with it while a second copy may still be about to set
// it.
var (
	theMutex windows.Handle
	theEvent windows.Handle
)

// alreadyRunning reports whether another copy is up, and asks it to bring its window to the
// front if so. A caller that gets true has nothing left to do.
func alreadyRunning() bool {
	// 事件必须先建，顺序不能反：第二份设完信号就关掉自己那份句柄，而对象随最后一个句柄消失，
	// 先有第一份握着它，信号才留得住。
	theEvent = createEvent()

	// 双击启动的只问一次；「重启」叫起来的那一份要等前一份真的放手。
	wait := time.Duration(0)
	if restarting {
		wait = restartWait
	}
	handle, first := takeInstanceMutex(instanceMutex, wait)
	if first {
		theMutex = handle
		return false
	}
	// 已经有人拿着它了：叫它出来，然后这一份没有别的事可做。
	windows.CloseHandle(windows.Handle(handle))
	askToShow()
	if theEvent != 0 {
		// 这一份用完了；让对象活下去的是第一份手里那个句柄。
		windows.CloseHandle(theEvent)
		theEvent = 0
	}
	return true
}

// takeInstanceMutex tries to be the first copy. It answers with the handle to keep for the
// life of the process, or with first=false when another copy holds the name.
//
// wait is how long to keep trying: zero is what a double click wants, and a few seconds is
// what 重启 needs, because the copy that started it is still on its way out. A mutex that
// cannot be created at all is reported as first=true with no handle - running without the
// lock beats not starting.
func takeInstanceMutex(name string, wait time.Duration) (windows.Handle, bool) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		slog.Error("single instance: cannot name the mutex", "error", err)
		return 0, true
	}
	deadline := time.Now().Add(wait)
	for {
		// 走 DLL 而不是 windows.CreateMutex：那个包装把 ERROR_ALREADY_EXISTS 折进它的
		// error 返回值里，而这里要找的答案正是那个 error。
		handle, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(ptr)))
		if handle == 0 {
			slog.Error("single instance: CreateMutex failed", "error", callErr)
			return 0, true
		}
		if callErr != windows.ERROR_ALREADY_EXISTS {
			return windows.Handle(handle), true
		}
		// 名字被别人拿着：关掉自己这个句柄，等一会儿再问。对象随最后一个句柄消失，所以前一份
		// 进程一结束，下一次问就是「还没有人」。
		windows.CloseHandle(windows.Handle(handle))
		if !time.Now().Before(deadline) {
			return 0, false
		}
		time.Sleep(restartPoll)
	}
}

// successorCommand is the command that starts the next copy, for 重启 to hand over to.
//
// HideWindow is deliberately unset, and hiddenCommand - which starts the service - is the other
// way round. Go turns HideWindow into STARTF_USESHOWWINDOW with SW_HIDE, and a GUI process
// ignores the argument of its *first* ShowWindow call in favour of STARTUPINFO: the copy would
// keep its window unshown until something asked for it. CREATE_NO_WINDOW is harmless for a GUI
// program and keeps a console from appearing if this is ever built without -H=windowsgui.
func successorCommand() (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, successorArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: creationNoWindow}
	return cmd, nil
}

// startSuccessor starts that command. The child is not waited for: on Windows it outlives this
// process, which still has a tray to take down and a service to stop. Nothing carries it off
// with the service either - the job object that ends the service when this program ends
// (containService) is given the service process alone, not this one.
func startSuccessor() error {
	cmd, err := successorCommand()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	slog.Info("restart: started the next copy", "path", cmd.Path, "args", cmd.Args[1:])
	return nil
}

// successorArgs is how the next copy is started: the restart marker always, and the arguments
// this copy was given that change what it does. -no-service is one of them - a copy told to
// keep its hands off the service must not hand over to one that starts a service of its own.
func successorArgs() []string {
	args := []string{restartFlag}
	if noService {
		args = append(args, "-no-service")
	}
	return args
}

// createEvent makes the event the first copy waits on, or opens the one it already made.
//
// Automatic reset and starting unsignalled are both deliberate: one SetEvent releases exactly
// one wait, and a copy that is first does not wake up with a request already pending against
// its own window. ERROR_ALREADY_EXISTS is not a failure - it is the second copy's way in, and
// the handle is good either way.
func createEvent() windows.Handle {
	name, err := windows.UTF16PtrFromString(instanceEvent)
	if err != nil {
		slog.Error("single instance: cannot name the event", "error", err)
		return 0
	}
	handle, err := windows.CreateEvent(nil, 0, 0, name)
	if handle == 0 {
		slog.Error("single instance: CreateEvent failed", "error", err)
		return 0
	}
	return handle
}

// askToShow asks the copy that is already up to bring its window to the front.
//
// By event rather than by a broadcast window message, which is what this used to be: a message
// sent before the window existed was missed, and repeating it landed after the user had closed
// the window (closing hides it). A signal that arrives before anyone waits is not lost, so
// setting it once is enough and repeating it is not wanted.
func askToShow() {
	if theEvent == 0 {
		slog.Warn("single instance: no event to ask the running copy with")
		return
	}
	if err := windows.SetEvent(theEvent); err != nil {
		slog.Warn("single instance: asking the running copy failed", "error", err)
		return
	}
	slog.Info("single instance: asked the running copy to come forward")
}

// watchShowRequests answers those requests for the life of the process: each time the event
// is set, the window comes forward.
//
// It starts once the window exists: a request that arrived earlier is still set when the first
// wait gets to it, which is the point of waiting on an event. The wait is on the window's own
// handle rather than a message loop, so Show lands on the thread that owns the window.
func watchShowRequests() {
	if theEvent == 0 {
		return
	}
	go func() {
		for {
			event, err := windows.WaitForSingleObject(theEvent, windows.INFINITE)
			if event != windows.WAIT_OBJECT_0 {
				slog.Warn("single instance: waiting for a show request ended", "error", err)
				return
			}
			slog.Info("single instance: another start asked for the window")
			win.Show()
		}
	}()
}
