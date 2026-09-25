//go:build windows

package main

// The DSH service behind the window: the command that starts it, and the one-time link it
// prints once it is up.
//
// The link matters because the token in it is minted per process. An address saved from the
// last run carries a token this run's service has never heard of, and opening it only gets a
// 401, so the token is never written down (stripToken) and the page is opened again when the
// line announcing the new one appears.

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// serviceAddress is where the service listens and where the window opens. Both use
	// the same host on purpose: dsh names its session cookie after the authority, so
	// localhost:3080 and 127.0.0.1:3080 are two different logins.
	serviceAddress = "http://127.0.0.1:3080/"

	// serviceWait is how long the service is given to come up. The first run of npx
	// downloads the package, and that is the slow part.
	serviceWait = 60 * time.Second
	servicePoll = 500 * time.Millisecond

	// creationNoWindow is CreateProcess's CREATE_NO_WINDOW. This program has no console of
	// its own, so a console program started without this flag is given a brand new console
	// window - the black box that flashes past. Every child process goes through
	// hiddenCommand because of it.
	creationNoWindow = 0x08000000
)

// noService is set by -no-service or --no-service: the window is then opened against
// whatever happens to be listening already, and nothing is started.
var noService bool

// The service's output is kept only long enough to find the line that carries the link.
// 32 KiB is far more than that line needs, and the cap keeps a talkative plugin from
// growing the buffer without end.
var (
	serviceMu     sync.Mutex
	serviceOutput bytes.Buffer
	serviceLink   string

	serviceAnnounce     = make(chan struct{})
	serviceAnnounceOnce sync.Once
)

// serviceLine matches the announcement dsh prints when it is ready, and nothing else in the
// output that merely looks like a URL.
//
// The trailing whitespace is part of the pattern on purpose: one write can end in the middle
// of the line, and half a link is half a token, which fails later as a 401 rather than here
// as something anyone could act on.
var serviceLine = regexp.MustCompile(`dsh web:[ \t]*(https?://[^\s"'<>]+)[ \t\r\n]`)

// serviceWriter passes the service's output to the log and watches it for that line.
type serviceWriter struct{}

func (serviceWriter) Write(p []byte) (int, error) {
	if text := bytes.TrimRight(p, "\r\n"); len(text) > 0 {
		slog.Info("service output", "line", text)
	}
	serviceMu.Lock()
	announced := false
	if serviceLink == "" {
		serviceOutput.Write(p)
		if serviceOutput.Len() > 32*1024 {
			kept := serviceOutput.Bytes()
			serviceOutput.Reset()
			serviceOutput.Write(kept[len(kept)-32*1024:])
		}
		if m := serviceLine.FindSubmatch(serviceOutput.Bytes()); m != nil {
			serviceLink = string(m[1])
			announced = true
		}
	}
	serviceMu.Unlock()
	if announced {
		serviceAnnounceOnce.Do(func() { close(serviceAnnounce) })
	}
	return len(p), nil
}

// announcedLink is the link the service printed, token and all, or "" while there is none.
func announcedLink() string {
	serviceMu.Lock()
	defer serviceMu.Unlock()
	return serviceLink
}

// serviceArgs is what dsh is started with.
//
// --no-open: without it the service opens the same link in the default browser, which is a
// second window nobody asked for.
var serviceArgs = []string{"-y", "@deepseek-ai/dsh", "web", "--no-open"}

// hiddenCommand is a child process that is never given a console window of its own.
func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: creationNoWindow}
	return cmd
}

// The service this program started, if it started one. Nothing else may be killed, and a
// service the user had running already is not ours to stop.
var (
	serviceCmd    *exec.Cmd
	serviceIsOurs bool
)

// startService makes sure something is listening on serviceAddress, starting the service if
// nothing is. It returns once the service has announced its link, or when serviceWait runs
// out.
func startService() {
	// 判断依据是那个地址**回什么**，而不是它回不回：别人的服务占着端口时，本次运行发出的每
	// 一个请求都会拿到 401，等它给一条链接，等的是一个不可能发生的事。
	switch probe(2 * time.Second) {
	case serviceReady:
		slog.Info("a service is already up and authorised", "address", serviceAddress)
		return
	case serviceUnauthorized:
		// 端口上有别人起的服务。这里**不下结论**：Go 侧的探测没有 cookie，看不出这个窗口能
		// 不能进去——它可能认得 profile 里那个会话 cookie。main 已经把地址打开了，
		// verifyWindowAccess（access.go）会带着 cookie 问一次，真被拒才由它换成提示页。
		// 这里如果也 setTrouble，就会把那边刚验证好的页面盖掉。
		slog.Info("the port is held by another service", "address", serviceAddress)
		return
	}

	slog.Info("starting the service", "command", "npx", "args", serviceArgs)
	cmd := hiddenCommand("npx", serviceArgs...)
	cmd.Stdout, cmd.Stderr = serviceWriter{}, serviceWriter{}
	if err := cmd.Start(); err != nil {
		// 在 Windows 上 npx 是个 .cmd，光用这个名字不一定找得到。
		slog.Warn("npx did not start, trying npx.cmd", "error", err)
		cmd = hiddenCommand("npx.cmd", serviceArgs...)
		cmd.Stdout, cmd.Stderr = serviceWriter{}, serviceWriter{}
		if err := cmd.Start(); err != nil {
			slog.Error("the service could not be started", "error", err)
			return
		}
	}
	serviceCmd, serviceIsOurs = cmd, true
	slog.Info("service started", "pid", cmd.Process.Pid)
	containService(cmd.Process.Pid)

	// 「就绪」是两件事都成立：端口有响应，而且链接已经打出来了。端口先起来——在拿到授权之
	// 前，dsh 对 / 一直回 401——这时候把窗口开到那个地址上，只会落到一个没有 token 的页面里。
	deadline := time.Now().Add(serviceWait)
	reported := false
	for time.Now().Before(deadline) {
		if listening() {
			if !reported {
				reported = true
				slog.Info("the port is listening, waiting for the link")
			}
			if announcedLink() != "" {
				slog.Info("the service is ready")
				return
			}
		}
		time.Sleep(servicePoll)
	}
	slog.Warn("no link was announced", "waited", serviceWait)
	setTrouble(unannouncedPage())
}

// listening reports whether anything answers on serviceAddress. It answers "is there a
// server", which is the question worth asking only once this run has started one itself.
func listening() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serviceAddress)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// serviceState is what the address answers: nothing, a service this run has no key to, or a
// service that will let this window in.
type serviceState int

const (
	serviceAbsent serviceState = iota
	serviceUnauthorized
	serviceReady
)

// probe asks the address one question: is anything listening there.
//
// It is a plain HTTP client, and that is the whole reason its answers have to be handled
// carefully: it carries no cookies. dsh answers 401 on / until the token in its link has
// been exchanged for a session cookie, so:
//
//	401 - there is a service, and it does not recognise *this client*. It may well recognise
//	      the window, whose cookies live in the browser profile and outlive the run that
//	      earned them. So a 401 is not a verdict on whether the page can be opened: it only
//	      rules out waiting here for a link, which a service this run did not start will
//	      never print.
//	2xx or 3xx (the code tests 200-399) - a service that knows even this cookieless client,
//	      which means the window can open the saved address for certain.
//	none - nothing is there; the service has to be started, and its link waited for.
func probe(timeout time.Duration) serviceState {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(serviceAddress)
	if err != nil {
		return serviceAbsent
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return serviceUnauthorized
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		return serviceReady
	default:
		// 别的任何回答，同样都是「不是本次运行能用的服务」。
		return serviceUnauthorized
	}
}

// What startService found that the window should say out loud. Empty means nothing to
// report; the value is a page of this app's own, built in notice.go.
var (
	troubleMu   sync.Mutex
	troublePage string
)

func setTrouble(page string) {
	troubleMu.Lock()
	troublePage = page
	troubleMu.Unlock()
}

func troubleURL() string {
	troubleMu.Lock()
	defer troubleMu.Unlock()
	return troublePage
}

// serviceJob holds the service this run started. Its one rule is
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: when the last handle to the job closes - which happens
// when this process ends, by any means at all, a crash and a kill from the Task Manager
// included - Windows ends every process in it.
//
// This is what keeps a crash from leaving a service behind. An orphaned service keeps
// answering on the port, and the next run then finds it taken, starts nothing, and waits for
// a link that will never be printed. Ending the tree on the way out makes that unreachable.
var serviceJob windows.Handle

// containService puts a started service under that job. Failure is logged and survived: the
// graceful path still stops the service, and a process that is already in a job this process
// may not modify refuses the assignment.
func containService(pid int) {
	if serviceJob == 0 {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			slog.Error("job object: cannot create it", "error", err)
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			slog.Error("job object: cannot set its limits", "error", err)
			windows.CloseHandle(h)
			return
		}
		serviceJob = h
	}
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		slog.Error("job object: cannot open the process", "pid", pid, "error", err)
		return
	}
	defer windows.CloseHandle(p)
	if err := windows.AssignProcessToJobObject(serviceJob, p); err != nil {
		slog.Error("job object: cannot assign the process", "pid", pid, "error", err)
		return
	}
	slog.Info("the service is in the job: it ends when this program does", "pid", pid)
}

// watchService opens the announced link as soon as it exists.
//
// The window was opened on the saved address, which cannot carry a usable token, so the
// page that works is the one the service has just printed. The call goes through Dispatch
// because this runs on a goroutine of its own and navigating is a WebView2 controller call,
// which belongs to the thread the message loop runs on.
func watchService() {
	go func() {
		<-serviceAnnounce
		if link := announcedLink(); link != "" {
			slog.Info("opening the link the service announced")
			win.Dispatch(setURL, link)
		}
	}()
}

// stopService ends the service, but only if this program started it. taskkill is used
// because the service is npx, a wrapper around node: ending the wrapper alone would leave
// the server running.
func stopService() {
	if !serviceIsOurs || serviceCmd == nil || serviceCmd.Process == nil {
		return
	}
	// 关掉 job 本身就结束了整棵树，连下面那条 taskkill 够不着的进程也一并结束。
	if serviceJob != 0 {
		windows.CloseHandle(serviceJob)
		serviceJob = 0
	}
	slog.Info("stopping the service")
	done := hiddenCommand("taskkill", "/PID", strconv.Itoa(serviceCmd.Process.Pid), "/T", "/F")
	done.Stdout, done.Stderr = io.Discard, io.Discard
	if err := done.Run(); err != nil {
		slog.Warn("stopping the service failed", "error", err)
	}
	serviceCmd, serviceIsOurs = nil, false
}

// stripToken removes the one-time token from an address.
//
// The token is minted by the service for one process, so a stored one is a key to a door
// that no longer exists: keeping it would mean every run begins by being turned away. The
// rest of the address is worth keeping, which is why this removes the parameter and not the
// query.
func stripToken(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	q := u.Query()
	if !q.Has("token") {
		return raw
	}
	q.Del("token")
	u.RawQuery = q.Encode()
	return u.String()
}
