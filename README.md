<div align="center">

<img src="internal/artwork/harness.svg" width="96" height="96" alt="DeepSeek Harness Desktop">

# DeepSeek Harness 桌面应用

把 DeepSeek Harness 的网页界面放进一个原生窗口，常驻系统托盘。

[![Windows](https://img.shields.io/badge/Windows-10%20%2F%2011-0078D4?logo=windows&logoColor=white)](#安装)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](go.mod)
[![WebView2](https://img.shields.io/badge/WebView2-Evergreen-0078D4)](#安装)
[![release](https://img.shields.io/github/v/release/Drelf2018/deepseek-harness-desktop?logo=github&label=release)](https://github.com/Drelf2018/deepseek-harness-desktop/releases)
[![MIT](https://img.shields.io/badge/license-MIT-3DA639)](LICENSE)

</div>

## 安装

到 [Releases](https://github.com/Drelf2018/deepseek-harness-desktop/releases) 下载最新一版的 `.exe`，双击运行。

> [!NOTE]
> 需要 **WebView2 运行时**：Windows 11 自带，Windows 10 大多随 Edge 装过。缺了它程序会直接报「缺少 WebView2 运行时」，而不是开出一个空窗口。

> [!TIP]
> 第一次运行要靠 `npx` 下载并启动 DSH 服务，所以需要 Node.js 和网络；之后用的是缓存，很快就起来。

## 使用

### 菜单分两处

```text
右键标题栏 / Alt+空格（系统菜单，插在最前面）
  置顶 / 居中 / 刷新
  ──────── 之后是系统自己的：还原 / 移动 / 大小 / 最小化 / 最大化
  恢复参数尺寸 / 参考尺寸 ▸ / 相对占比 ▸ / 宽高比 ▸
  ──────── 最后仍是系统自己的：关闭

托盘（平铺）
  显示窗口 / 开机自启动
  ────────────────
  设置访问地址 / 在浏览器中打开
  ────────────────
  打开日志文件 / 打开数据目录 / 打开程序目录
  ────────────────
  重启 / 退出
```

本地构建（`Version` 仍是 `dev`）在「打开程序目录」那组之后还多一个**测试**子菜单：三张提示页平时要等真实故障才看得到，那里可以直接把它们调出来看效果；最后一项「系统通知」直接弹一条通知，用来验通知那条链（`notify.ps1` → PowerShell → 按 `appID` 署名 → 点它把窗口叫回来）。发布版没有它——`-X main.Version=<tag>` 每次都会把 `Version` 填上，所以 `"dev"` 就等于「这一份是自己编的」。

### 重启是什么

「重启」是退出加一次重新开始：它先把接替的那一份叫起来（带上 `-restart`，让它等这一份松开单实例 mutex），再走完正常的退出——记住尺寸与地址、结束自己启动的服务。于是窗口按上次的大小和地址重新开出来，服务也是新的一份（那条链接上的会话随之结束）。原样传下去的只有影响行为的参数：`-no-service`。

### 命令行开关

| 开关 | 作用 |
|---|---|
| `-no-service` / `--no-service` | 不启动服务，只开一个窗口对着已经在监听的地址 |
| `-restart` / `--restart` | 接替者用的内部开关：等前一份松开单实例 mutex（`instance.go`），自己双击时不必带 |

## 构建

需要 Go 1.27 或更高。不需要 C 工具链，也不需要 `rsrc.exe` —— 图标资源由本仓库里的程序自己写出来。

```powershell
go run ./internal/genicon                   # 生成 rsrc.syso
go build -ldflags=-H=windowsgui -o "DeepSeek Harness Desktop.exe" .
```

- **`rsrc.syso`** 是 exe 里的图标资源：资源管理器、快捷方式、固定到任务栏之后读的都是它。它**不进仓库**，由 `internal/genicon` 现做现用。少了这一步构建照样成功，只是图标静默退回系统默认，没有任何警告——所以 `go build` 之前先跑上面那一条。
- **`-ldflags=-H=windowsgui`**：默认的控制台子系统会在窗口旁边多开一个控制台，它的任务栏按钮用的是系统控制台图标，看起来就像这个程序没有图标。它还有一个更晚才发现的后果：**点通知会闪一个 cmd**——点通知等于再拉起一份本程序，而那一份在控制台子系统下会被 Windows 新开一个控制台窗口（第一份没有，只是因为它继承了启动它的那个终端）。`go run .` 不带这个标志，所以本地用 `go run` 调的时候很容易看到这一下；正式构建没有。

## 发布

由 GitHub Action 发布：先由 `pre_command` 跑 `go run ./internal/genicon amd64`，再带 `-ldflags "-H=windowsgui -X main.Version=<tag>"` 构建上传。版本只在启动时写进日志。

## 项目结构

| 文件 | 管什么 |
|---|---|
| `main.go` | 程序本身：名字、菜单、启动与退出的顺序 |
| `window.go` | WebView2 窗口和它下面的 Win32：窗口矩形/可见框的换算、移动、置顶，以及窗口自己的图标（标题栏、任务栏、Alt+Tab） |
| `dispatch.go` | 窗口自己的线程：消息过程与 `Dispatch` 队列（刷新用的 `reload.js` 也在这里） |
| `access.go` | 窗口带着 cookie 问一次进不进得去：`askWindow.js` 与那 5 秒的等待 |
| `sysmenu.go` | 窗口系统菜单上那一段：置顶/居中/刷新/恢复参数尺寸 + 三组尺寸 |
| `size.go` | 三组尺寸菜单描述的那个尺寸，以及它的下限 |
| `service.go` | DSH 服务的启动、输出解析、带 token 链接、Job Object 兜底 |
| `notice.go` + `loading.html` | 服务不可用时的三张本地提示页：文字在 Go 里，版式在模板里，转义交给 `html/template` |
| `notify.go` + `notify.ps1` | 等人的面板出现时弹系统通知：注册 AppUserModelID 与 URL 协议；toast 连载荷一起写在那个 .ps1（`text/template`）里，Go 只填启动地址、署名与两行文字 |
| `instance.go` | 单实例：互斥体定谁是第一份，具名事件把第二次启动叫窗口的请求交给它 |
| `startup.go` | 开机自启动（注册表 `HKCU\...\Run`） |
| `appdata.go` | 本程序的路径：`%LOCALAPPDATA%` 下的日志与窗口状态，以及 exe 所在的程序目录 |
| `js/` | 注入到页面里的四段脚本：`askForURL.js`（改地址）、`askWindow.js`（问一次进不进得去）、`reload.js`（刷新）、`notify.js`（出现等人的面板时通知一声）。各自的 `go:embed` 就在用它的人旁边 |
| `internal/artwork/` | 图标源按任意尺寸画出来、居中，拼成多尺寸 .ico，或只装一张（通知的图标要的是文件，而且只能装一张）。**与平台无关**，所以生成器能在 Linux 容器里跑 |
| `internal/artwork/harness.svg` | 图标源，矢量，每个尺寸现画 |
| `internal/genicon/` | 生成器：写 `rsrc.syso`（`go run ./internal/genicon [arch]`） |
| `icon_test.go` | 只校验：把同样几张图读回来，检查条目数与尺寸，不写任何文件 |
| `*_test.go` | 尺寸下限、自启动、提示页、服务输出解析、系统菜单 id |

## 实现细节

<details>
<summary>窗口尺寸：三组参数与那个下限</summary>

菜单算出来的尺寸有下限（内容需要 480x360 CSS 像素，按显示缩放换算成设备像素）：「屏幕高度 + 1/3 + 9:16」这种组合能算出 256 像素宽，比页面能用的还窄，所以在 `size.go` 里夹住。鼠标拖拽的下限用同一个数，通过库的 `SetSize(..., HintMin)` 交给 Windows。

</details>

<details>
<summary>别人的服务占着端口时，怎么判断进不进得去</summary>

Go 侧的探测没有 cookie，看不出这个窗口能不能进去，所以不直接给错误页：先照常打开地址，再由窗口带着 profile 里的会话 cookie 问一次（`js/askWindow.js` → `_probe`）——只有真被拒（401/403）才换成提示页。

</details>

<details>
<summary>面板等人的时候，系统通知是怎么来的</summary>

审批、提问、计划评审这三块面板只有页面自己看得见，所以由 `js/notify.js` 盯着：它随每个文档装进去（`Init`），用 `MutationObserver`（外加 1.5 秒一次的兜底）看那三个 `data-*-key` 属性有没有出现，出现一个新的就调用 `window._notify(标题, 正文)`。去重靠的就是那个 key——同一个面板重绘多少次都只发一条，面板消失之后 key 被忘掉，所以同一块面板再来一次会再通知一次。标题与正文取自面板自己的文字（审批取 scroll 里的第一行，提问取 `h2`，计划评审取第一个标题），裁到 120 字。

Go 那半边在 `notify.go`：一个绑定接住这次调用，把它渲染进 `notify.ps1`（那个文件是 `text/template`，留着 `{{.Launch}}` / `{{.Notifier}}` 和两处文字；`escapeXML` 注册成模板函数 `xml`，负责把文字转义进 XML 载荷），再交给 Windows PowerShell 5.1 的 WinRT toast——Go 自己够不着那个 API，为一条通知不值得拉一整套 COM。转义不能省：一个没转义的 `&` 就足以让通知悄悄不出现；脚本本身走 `-EncodedCommand`（UTF-16LE base64），因为命令行要经过进程的 ANSI 代码页，中文到那里就不是原来的字符了。

通知要像这个程序自己发的、点了要能回到这个窗口，所以启动时写三处注册表：`HKCU\Software\Classes\AppUserModelId\<appID>` 下的 `DisplayName`（没有它，通知会署名成 Windows PowerShell）与 `IconUri`（那幅画得先落到数据目录里，因为 shell 读的是文件，不是字节；用**单尺寸的 .ico**，尺寸取 `SM_CXSMICON`，与托盘图标是同一份字节。标题旁边那个位置**不是**文档里 `appLogoOverride` 的 48，那个是正文左边的缩略图；而多尺寸那份交给它会被画糊、还啃掉边缘，所以只装一张——`notify.go` 里 `notificationIcon` 的注释写着这段来历），以及 `<appID>://` 这个 URL 协议。署名旁边那幅画就来自这份注册。**这份注册 shell 只在它第一次见到这个 AUMID 时读一次**——先注册 `DisplayName`、后补 `IconUri` 的话，通知会一直「有名字、没图标」——重启 explorer 或注销一次才恢复。图**不**放在载荷里：`appLogoOverride` 那个槽位是正文左边的缩略图，不是标题位置，两处不是一回事。点通知就是让该协议把程序再叫起来一次：那一份发现单实例互斥体有人拿着，就让已经在跑的那份把窗口提到前面（`instance.go`），自己什么都不做。

</details>

<details>
<summary>日志的写法</summary>

全程序只用 `log/slog`，出口只有一个：`setupLogging` 里那一句 `slog.SetDefault`（它顺带把标准 `log` 包桥接过来，所以库在 `systray.Logger` 为 nil 时写的行也落在同一个文件里）。写日志时：

- **消息是稳定的模板，变量进字段**：`slog.Info("service started", "pid", pid)`，而不是 `log.Printf("service pid %d", pid)`。前者的每一行都是同一条 `msg`，可以按 `msg=` 检索、按字段过滤；后者每次都是一条新消息，既数不出来也过滤不了。
- **级别按语义**：`Error` 是「这件事失败了」，`Warn` 是「还能继续，但值得知道」（服务没起来、排队的活儿迟了 48 秒、自启动没写成），`Info` 是启动与状态。
- **字段用词固定**：`error`、`path`、`url`、`pid`、`status`、`waited`、`late`、`line`、`title`；`windowState` 自己实现了 `LogValue`，于是落成 `state.width=726 state.onTop=false` 这样的分组。

</details>

## 数据与日志

都在 `%LOCALAPPDATA%\deepseek-harness-desktop\`：

| 路径 | 是什么 |
|---|---|
| `window-state.json` | 窗口大小、是否最大化、上次打开的地址（**不含 token**）；位置有意不记 |
| `app.log` | 运行日志，超过 1 MiB 轮转一次 |
| `notification-icon.ico` | 通知里署名旁边那幅画：`IconUri` 要文件，所以每次运行现画一张放在这里；**只装一张**，尺寸取 `SM_CXSMICON` |
| `EBWebView\` | WebView2 的浏览器配置目录，会话 cookie 在这里 |

## 致谢

| 依赖 | 在这里做什么 |
|---|---|
| [Drelf2018/systray](https://github.com/Drelf2018/systray) | 托盘图标与菜单 |
| [jchv/go-webview2](https://github.com/jchv/go-webview2) | WebView2 窗口 |
| [Drelf2018/oksvg](https://github.com/Drelf2018/oksvg) + [srwiley/rasterx](https://github.com/srwiley/rasterx) | SVG 按尺寸光栅化 |
| [akavel/rsrc](https://github.com/akavel/rsrc) + [biessek/golang-ico](https://github.com/biessek/golang-ico) | 写 PE 图标资源 |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | Win32 与 COM 调用 |

## 许可证

[MIT](LICENSE) © 2026 Drelf2018
