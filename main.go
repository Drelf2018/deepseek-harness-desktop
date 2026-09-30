//go:build windows

// The app puts a tray icon and a WebView2 window together. The menu is split by what it acts on:
// the window carries its own (置顶, 居中, 刷新, 恢复参数尺寸 and the three size settings) in the
// system menu on the title bar, the tray carries the program's (显示窗口, the address, the log,
// autostart, 重启, 退出).
//
// The window is up as soon as it is created, and closing it hides it rather than quitting: the
// tray stays, and 显示窗口 brings it back, until 退出 is picked. Its size and address are
// remembered between runs - the position is not, since a run starts centred.
//
// Built with -ldflags=-H=windowsgui: the console subsystem would put a console window beside it,
// whose taskbar button wears the console icon. README.md has the two build steps.
package main

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Drelf2018/deepseek-harness-desktop/internal/artwork"
	"github.com/Drelf2018/systray"
	"golang.org/x/sys/windows"
)

// The app names itself twice, and the two are not interchangeable.
//
// appName is what a person reads: the window title, the tray tooltip, the caption of
// an error box - and, in the full app, the DisplayName the shell shows as the source
// of a notification.
//
// appID is what a machine reads, and what the app is filed under: the folder under
// %LOCALAPPDATA%, and - in the full app - the AppUserModelID and the URL scheme. It
// is ASCII, lowercase, and must never change: the day it does, every user's saved
// window position, browser profile and single instance mutex are left behind under
// the old name. Do not derive it from appName.
const (
	appName = "DeepSeek Harness 桌面应用"
	appID   = "deepseek-harness-desktop"
)

// Version is the build's version, and the one thing in this program the linker rewrites: the
// release workflow builds with
//
//	-ldflags "-H=windowsgui -X main.Version=${{ github.ref_name }}"
//
// so a release carries the tag it was published from, and a local build stays at "dev".
//
// It has to stay a plain string variable: -X only reaches one whose initializer is a constant
// expression, and does nothing at all - silently - to a variable computed at run time.
//
// Nothing in the menu shows it. It goes to the log at startup, which is where the question it
// answers ("which build is this") is asked anyway.
var Version = "dev"

// defaultURL is what a run opens when nothing has been chosen yet: the service's own page,
// which is what this window is a front end for. Another address can be set from the menu at
// any time, and that one is remembered instead.
const defaultURL = serviceAddress

// win is the main window, set once in main and never nil after that: newWindow
// returns nil when the WebView2 runtime is missing or the window could not be
// created, and main stops there rather than running a tray with nothing behind it.
var win *webviewWindow

func showError(cause string) error {
	title, err := windows.UTF16PtrFromString(appName)
	if err != nil {
		return err
	}
	body, err := windows.UTF16PtrFromString(cause)
	if err != nil {
		return err
	}
	_, err = windows.MessageBox(0, body, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_TASKMODAL|windows.MB_SETFOREGROUND)
	return err
}

// The address the window is showing. The tray menu's goroutines read it, the page's
// binding writes it, and both end up on the thread that runs the message loop.
var (
	urlMu      sync.RWMutex
	currentURL = defaultURL
)

// url is the address the window is showing, or is about to.
func url() string {
	urlMu.RLock()
	defer urlMu.RUnlock()
	return currentURL
}

// address turns what was typed into an address the window can open.
//
// What gets typed is usually a host and maybe a port with nothing in front of it, and
// WebView2 reads that as a search rather than as a page. Anything that does not bring its
// own scheme gets http:// in front of it; a scheme is what has :// in it, or one of the few
// names that are written without slashes.
func address(typed string) string {
	target := strings.TrimSpace(typed)
	if target == "" {
		return ""
	}
	if strings.Contains(target, "://") {
		return target
	}
	lower := strings.ToLower(target)
	for _, prefix := range []string{"about:", "data:"} {
		if strings.HasPrefix(lower, prefix) {
			return target
		}
	}
	return "http://" + target
}

// openStartupURL points the window at the address the last run was left showing, or at the
// default when there is none. It records nothing: the file on disk belongs to the previous
// run, and this run writes its own when it ends.
func openStartupURL(saved string) {
	target := address(saved)
	if target == "" {
		target = defaultURL
	}
	urlMu.Lock()
	currentURL = target
	urlMu.Unlock()

	slog.Info("opening the saved address", "url", target)
	win.Navigate(target)
}

// showNotice puts a page this app built into the window, and remembers nothing: the address
// the user chose is still the address the next run should open. Navigating is a WebView2
// controller call, so it goes through Dispatch.
func showNotice(page string) {
	if win == nil || page == "" {
		return
	}
	slog.Info("showing a page of our own", "page", page[:min(len(page), 48)])
	win.Dispatch(win.Navigate, page)
}

// setURL points the window at a new address and remembers it. This is what the page calls,
// so it arrives on the thread that runs the message loop, where Navigate and the window
// rectangle both belong.
func setURL(raw string) {
	target := address(raw)
	if target == "" {
		return
	}
	urlMu.Lock()
	currentURL = target
	urlMu.Unlock()

	slog.Info("opening a new address", "url", target)
	win.Navigate(target)
	remember()
}

// remember writes down the size the window is at and the address it is showing. A window
// whose rectangle cannot be read leaves the file alone, rather than writing a size of zero
// over a good one.
//
// The token is stripped on the way out: it belongs to this run's service and would be a
// rejected key on the next one.
func remember() {
	s := windowState{URL: stripToken(url()), Size: currentSize().onDisk(), OnTop: win.TopMost()}
	rect, ok := win.WindowRect()
	if !ok {
		slog.Warn("window state not saved: no window rectangle to read")
		return
	}
	s.Width, s.Height, s.Maximized = rect.Width, rect.Height, rect.Maximized
	saveWindowState(s)
}

// askForURLJS is the prompt for a new address: a page script rather than a Go string, because the
// answer has to come back through the _setURL binding - Eval throws a script's value away. The
// name the script declares is the name addMenuItems calls.
//
//go:embed js/askForURL.js
var askForURLJS string

func main() {
	// 放在最前面，这样下面的一切——包括库自己报的错——都会留下痕迹，之后有人能读到。
	setupLogging()

	// 版本只进日志：它是「这一份是哪一版」唯一的答案，而日志正是排障时第一个被打开的地方。
	slog.Info("starting", "version", Version)

	// -no-service：开一个窗口看着已经在监听的那个服务，不为它启动任何东西。
	// -restart：「重启」叫起来的接替者，前一份还在退出，要等它放手（instance.go）。别的参数都
	// 留给系统处理。
	for _, arg := range os.Args[1:] {
		if arg == "-no-service" || arg == "--no-service" {
			noService = true
		}
		if arg == restartFlag || arg == "--restart" {
			restarting = true
		}
	}

	// 只留一份：下面这些会建起托盘、窗口和服务，而第二次双击要的从来不是第二套。
	if alreadyRunning() {
		return
	}

	// 只在 Windows 上，而且必须在任何窗口出现之前：不调的话，缩放过的显示器报出来的图标
	// 度量还是 16 像素，系统只好把结果拉伸。
	if err := systray.EnableDPIAwareness(); err != nil {
		slog.Warn("dpi awareness", "error", err)
	}

	// 系统通知：注册 AUMID 与 URL 协议，并把通知那幅画按 Windows 说的尺寸现画一张（notify.go）。
	// 画尺寸要等 DPI 感知开好之后再问，所以放在这里；放在单实例之后，是因为被一条通知叫起来的
	// 那一份除了把窗口叫出来什么都不该做，不该再写一遍注册表。
	go prepareNotifications()

	// 托盘图标来自内置图形：凡是有可能被要到的尺寸都现画一张，打包成一个 .ico（internal/artwork）。
	// 托盘只要一张，就是通知区域此刻要的那个尺寸——所以这里不必预置一整张尺寸表，125% 缩放要 20
	// 像素时也不会退到 24 让系统去缩。需要那张表的是 exe 的资源图标，它只能构建时定。
	icon, err := artwork.IconICO(smallIconSize())
	if err != nil {
		slog.Error("icon", "error", err)
		err = showError("无法加载内置图标。")
		if err != nil {
			slog.Error("show", "error", err)
		}
		os.Exit(1)
	}

	// 默认尺寸：参考屏幕宽度，占比按屏幕宽度挑，宽高比 4:3。菜单在启动时显示的就是这一套，
	// 所以先把它交给菜单——菜单才会勾在正确的位置上，之后点任何一项也才有数可算。窗口尺寸
	// 优先用记住的那个。
	//
	// 菜单算出来的是**可见**尺寸，创建窗口要的是 window rect：有记录时不用换算（文件里存的
	// 本来就是 window rect），第一次运行则先量一次边框再换算，这样第一帧就是菜单承诺的尺寸。
	screenW, screenH := displaySize()
	preset := sizeState{anchor: screenWidth, share: defaultShare(screenW), ratio: ratio{4, 3}}

	// 文件只读一次：它同时带着窗口矩形和三组参数。参数在这里就要装回菜单，菜单构建时
	// 才能勾对位置。
	saved, haveState := loadWindowState()
	if haveState {
		if s, ok := saved.Size.state(); ok {
			preset = s
		}
	}
	storeSize(preset)

	var state windowState
	if haveState {
		state = saved
	} else if edges, ok := measureFrameEdges(); ok {
		width, height := preset.windowSize(screenW, screenH)
		state.Width = width + int(edges.left+edges.right)
		state.Height = height + int(edges.top+edges.bottom)
		slog.Info("first run: frame measured",
			"edgeLeft", edges.left, "edgeTop", edges.top, "edgeRight", edges.right, "edgeBottom", edges.bottom,
			"windowWidth", state.Width, "windowHeight", state.Height,
			"visibleWidth", width, "visibleHeight", height)
	}

	win = newWindow(state)
	if win == nil {
		slog.Error("webview2: the runtime is missing, or the window could not be created")
		err = showError("缺少 WebView2 运行时。")
		if err != nil {
			slog.Error("show", "error", err)
		}
		os.Exit(2)
	}

	// 窗口自己的图标：任务栏和 Alt+Tab 画的是它，跟 exe 里那份资源是两回事。
	if err := win.setWindowIcons(); err != nil {
		slog.Warn("window icons", "error", err)
	}

	// 第二份程序可能已经在敲门了：窗口一存在就把它接进来（instance.go）。等号右边那个事件比
	// mutex 建得还早，所以敲在窗口出现之前的请求不会丢，只是要等到这里才被应答。
	watchShowRequests()

	// 窗口自己的四项（置顶/居中/刷新/恢复参数尺寸）接在系统菜单上：右键标题栏或
	// Alt+空格，见 sysmenu.go。
	// 上次是不是置顶，装回去：先落到窗口上，再装菜单——菜单上那个勾是从窗口读的。
	if state.OnTop {
		win.setTopMost(true)
	}
	installSystemMenu(win)

	// 页面把地址交回来靠的是这个绑定：Eval 不回传值。桥接脚本是随页面一起装进去的，
	// 所以绑定必须赶在 Run 之前注册。
	if err := win.Bind("_setURL", func(raw string) { setURL(raw) }); err != nil {
		slog.Warn("bind _setURL failed", "error", err)
	}

	// 窗口自己问「能不能进」时，状态码从这条路回来（access.go）。非阻塞：晚到的回话不该把
	// 那条线程堵住。
	if err := win.Bind("_probe", func(status int) {
		select {
		case accessAnswer <- status:
		default:
		}
	}); err != nil {
		slog.Warn("bind _probe failed", "error", err)
	}

	// 页面出现等人的面板（审批、提问、计划评审）时弹一条系统通知：页面那一半（js/notify.js）、
	// 接它的绑定、以及通知怎么变成一条命令，都在 notify.go。和上面两个绑定一样，必须赶在 Run 之前。
	installNotifications()

	// 窗口背后那个服务：没人在监听时在这里启动，并盯着它公布的那条链接——只有那个地址带得动
	// 可用的 token。启动是阻塞的（第一次 npx 要下载包），所以这期间窗口已经起来、显示着记住的
	// 地址。
	// 窗口先开哪个地址，取决于那个地址回什么：闭着眼睛打开记住的地址，正是让一次运行看起来坏
	// 掉的原因——不带 token 的地址回 401，别人的服务占着端口也回 401。两种情况都显示本程序自己
	// 的一页，真正的页面一出现就把它换掉。
	if noService {
		openStartupURL(state.URL)
	} else {
		switch probe(700 * time.Millisecond) {
		case serviceReady:
			openStartupURL(state.URL)
		case serviceUnauthorized:
			// 端口上有服务，但不是本次启动的那个。它可能认得这个窗口的 cookie，也可能不认
			// ——那只有窗口自己知道，所以先照常打开地址，再让它问一次（access.go）：真被
			// 拒（401/403）才换成提示页，进得去就什么都不做。
			openStartupURL(state.URL)
			verifyWindowAccess()
		default:
			showNotice(startingPage())
		}

		go func() {
			startService()
			if page := troubleURL(); page != "" {
				showNotice(page)
			}
		}()
		watchService()
	}

	onReady := func() {
		systray.SetIcon(icon)
		systray.SetTooltip(appName)
		systray.SetOnLeftClick(win.Show)
		addMenuItems()
	}

	// WebView2 自己跑消息循环，而托盘的隐藏窗口挂在同一个线程上，它的消息就由
	// 那个循环分发出去 —— 这正是 Register 存在的理由。用 Run 会和 WebView2 抢循环。
	systray.Register(onReady, nil)
	win.Run()
	slog.Info("the message loop ended")

	// 退出前把窗口尺寸和地址记下来。窗口最大化着退出也记得对：WindowRect 取的是
	// 「还原后的那个矩形」，并把最大化这件事一起记下。
	remember()
	// 服务跟着窗口一起走：把它留下，就等于让一个屏幕上已经没有前端对着的服务器继续跑。本程序
	// 启动之前就已经在跑的服务不是我们的，stopService 不会碰它。
	stopService()
	// 下面不能再有任何东西绕过这里结束进程：本程序里每个 os.Exit 都发生在 win 存在之前，也正
	// 因为这样，Destroy 可以就是一个放在最后的普通调用，而不必 defer。
	win.Destroy()
}

// addMenuItems builds the menu. systray appends in call order, so these calls run
// top to bottom exactly as the menu is drawn:
//
//	显示窗口
//	开机自启动
//	────────────────
//	设置访问地址
//	在浏览器中打开
//	────────────────
//	打开日志文件
//	打开数据目录
//	打开程序目录
//	────────────────
//	测试 ▸	（本地构建才有）
//	────────────────
//	重启
//	退出
func addMenuItems() {
	mShow := systray.AddMenuItem("显示窗口", "显示主窗口")
	go func() {
		for range mShow.ClickedCh {
			win.Show()
		}
	}()

	// 开机自启动紧随显示窗口。它是这张菜单里唯一的状态（勾选框），摆在最上面，勾没勾一眼
	// 就看得见，不必为了确认这一点把菜单读到底。
	//
	// 勾的是注册表里的真实状态，不是记在内存里的意图：setAutostart 写完之后会再读一次，
	// 写不进去（策略、权限）时勾就上不去，用户一眼能看出来没生效。
	mStartup := systray.AddMenuItemCheckbox("开机自启动", "登录时自动运行本程序", autostartEnabled())
	go func() {
		for range mStartup.ClickedCh {
			if setAutostart(!autostartEnabled()) {
				mStartup.Check()
			} else {
				mStartup.Uncheck()
			}
		}
	}()

	// 窗口那几项（置顶 / 居中 / 刷新 / 恢复参数尺寸）和三组尺寸参数搬到了窗口自己的
	// 菜单里（右键标题栏、Alt+空格，见 sysmenu.go）：它们调的就是这个窗口，跟托盘没关系。
	// 窗口的事跟着窗口走，托盘只留这个程序自己的事。
	systray.AddSeparator()

	// 这一段是这条地址：改它，或者拿它到别处打开。它排在「文件」之前，一是它比翻日志常用，
	// 二是这样一来它就不夹在几个「打开…」中间——下面那三个是同一件事，本地的文件与文件夹。
	mSetURL := systray.AddMenuItem("设置访问地址", "在弹出的输入框里改掉当前打开的地址")
	go func() {
		for range mSetURL.ClickedCh {
			// The window is raised first, and the script is handed to the window's thread, because Eval
			// ends in a WebView2 controller call that a menu goroutine must not make. An unchanged or empty
			// answer is dropped rather than navigating where the window already is.
			win.Show()
			win.Dispatch(win.w.Eval, fmt.Sprintf("%saskForURL(%q);", askForURLJS, url()))
		}
	}()

	mOpen := systray.AddMenuItem("在浏览器中打开", "用系统默认浏览器打开当前地址")
	go func() {
		for range mOpen.ClickedCh {
			shellOpen(url())
		}
	}()

	systray.AddSeparator()

	// 这一段是这个程序的东西都放在哪儿：它的日志、它写状态的地方、它自己被解开放着的地方。
	// 三个都是「打开…」，是同一件事，所以排在一起。
	mLog := systray.AddMenuItem("打开日志文件", "用默认程序打开运行日志")
	go func() {
		for range mLog.ClickedCh {
			shellOpen(logPath())
		}
	}()

	mData := systray.AddMenuItem("打开数据目录", "打开存窗口状态和日志的那个目录")
	go func() {
		for range mData.ClickedCh {
			shellOpen(appDataDir())
		}
	}()

	// 这一个和上面那个是两处地方，不该混：数据目录在 %LOCALAPPDATA% 下，是这个程序写东西的地方；
	// 程序目录是这个 .exe 被解开放着的地方，属于把它放在那里的人（见 appdata.go）。
	// 「程序」不是随手挑的词：原来叫「运行目录」，而「运行」在这个菜单里什么都不特指。
	mProgramDir := systray.AddMenuItem("打开程序目录", "打开本程序 exe 所在的那个目录")
	go func() {
		for range mProgramDir.ClickedCh {
			dir, err := programDir()
			if err != nil {
				slog.Warn("program dir", "error", err)
				continue
			}
			shellOpen(dir)
		}
	}()

	// 工作区是 dsh 自己的那一组目录，不属于本程序，所以它是一个子菜单而不是第四行：名字就是侧栏
	// 里看到的那个名字，点了就是「用默认程序打开那个文件夹」（见 workspace.go）。
	//
	// 读不到 registry 就不建这一项——没装 dsh、家被 DSH_HOME 指到了别处、文件换了格式，都是同一个
	// 回答：菜单里少一行，好过一个点了什么也不发生的空子菜单。
	if list := workspaces(); len(list) > 0 {
		mWorkspaces := systray.AddMenuItem("打开工作区目录", "打开某个 DeepSeek Harness 工作区所在的目录")
		for _, w := range list {
			item := mWorkspaces.AddSubMenuItem(w.Title, w.Path)
			path := w.Path
			go func() {
				for range item.ClickedCh {
					shellOpen(path)
				}
			}()
		}
	}

	systray.AddSeparator()

	// 本地构建里多一个「测试」子菜单：三张提示页要等真实的故障才看得到，通知要等一个真面板，
	// 这里都能直接调出来看。
	// 判断用 Version —— 发布时 -X main.Version=<tag> 一定把它填上，所以只有自己编的那一份还是 "dev"。
	if Version == "dev" {
		mTest := systray.AddMenuItem("测试", "调出三张本地提示页与一条测试通知")
		for _, c := range []struct {
			title   string
			tooltip string
			page    func() string
		}{
			{"正在启动", "服务启动时显示的那一张", startingPage},
			{"端口被占", "端口上已经有别人的服务时显示的那一张", blockedPage},
			{"服务没给链接", "服务启动了但没公布链接时显示的那一张", unannouncedPage},
		} {
			item := mTest.AddSubMenuItem(c.title, c.tooltip)
			page := c.page
			go func() {
				for range item.ClickedCh {
					win.Show()
					showNotice(page())
				}
			}()
		}

		// 通知那一项不与那三张页同形：它没有页面可显示，它把面板会走的那条路自己走一遍——
		// 渲染 notify.ps1、拉起 PowerShell、按 appID 署名。看见通知，就说明这条链是通的；
		// 点一下，还能一并验「点通知把窗口叫回来」那一段。
		mNotify := mTest.AddSubMenuItem("系统通知", "弹一条通知，看它长什么样、点它能不能把窗口叫回来")
		go func() {
			for range mNotify.ClickedCh {
				// 与绑定那一侧同理：不在这条 goroutine 上等 PowerShell 起落。
				go fireToast("测试通知", "审批、提问、计划评审出现时走的就是这条路。点这条通知，应当把窗口叫到前面。")
			}
		}()
	}

	// 接替的那一份先起来，这一份再走：反过来的话，新的一份会看见一个正在退出的旧实例，把它
	// 当成「已经有一份在跑」，叫它出来然后自己退掉——重启的结果会是一个窗口都没有。
	mRestart := systray.AddMenuItem("重启", "结束本程序并重新启动，它启动的服务一并重启")
	go func() {
		<-mRestart.ClickedCh
		if err := startSuccessor(); err != nil {
			slog.Error("restart: cannot start the next copy", "error", err)
			return
		}
		systray.Quit()
	}()

	mQuit := systray.AddMenuItem("退出", "退出程序")
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}
