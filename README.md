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

本地构建（`Version` 仍是 `dev`）在「打开程序目录」那组之后还多一个**测试**子菜单：三张提示页平时要等真实故障才看得到，那里可以直接把它们调出来看效果。发布版没有它——`-X main.Version=<tag>` 每次都会把 `Version` 填上，所以 `"dev"` 就等于「这一份是自己编的」。

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
- **`-ldflags=-H=windowsgui`**：默认的控制台子系统会在窗口旁边多开一个控制台，它的任务栏按钮用的是系统控制台图标，看起来就像这个程序没有图标。

## 发布

由 GitHub Action 发布：先由 `pre_command` 跑 `go run ./internal/genicon amd64`，再带 `-ldflags "-H=windowsgui -X main.Version=<tag>"` 构建上传。版本只在启动时写进日志。

## 项目结构

| 文件 | 管什么 |
|---|---|
| `main.go` | 程序本身：名字、菜单、启动与退出的顺序 |
| `window.go` | WebView2 窗口和它下面的 Win32：窗口矩形/可见框的换算、移动、置顶 |
| `dispatch.go` | 窗口自己的线程：消息过程与 `Dispatch` 队列（刷新用的 `reload.js` 也在这里） |
| `access.go` | 窗口带着 cookie 问一次进不进得去：`askWindow.js` 与那 5 秒的等待 |
| `sysmenu.go` | 窗口系统菜单上那一段：置顶/居中/刷新/恢复参数尺寸 + 三组尺寸 |
| `size.go` | 三组尺寸菜单描述的那个尺寸，以及它的下限 |
| `service.go` | DSH 服务的启动、输出解析、带 token 链接、Job Object 兜底 |
| `notice.go` + `loading.html` | 服务不可用时的三张本地提示页：文字在 Go 里，版式在模板里，转义交给 `html/template` |
| `instance.go` | 单实例：互斥体定谁是第一份，具名事件把第二次启动叫窗口的请求交给它 |
| `startup.go` | 开机自启动（注册表 `HKCU\...\Run`） |
| `appdata.go` | 本程序的路径：`%LOCALAPPDATA%` 下的日志与窗口状态，以及 exe 所在的程序目录 |
| `icons.go` | 窗口自己的图标：标题栏、任务栏、Alt+Tab |
| `js/` | 注入到页面里的三段脚本：`askForURL.js`（改地址）、`askWindow.js`（问一次进不进得去）、`reload.js`（刷新）。各自的 `go:embed` 就在用它的人旁边 |
| `internal/artwork/` | 图标源按任意尺寸画出来、居中，拼成多尺寸 .ico。**与平台无关**，所以生成器能在 Linux 容器里跑 |
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
<summary>日志的写法</summary>

全程序只用 `log/slog`，出口只有一个：`setupLogging` 里那一句 `slog.SetDefault`（它顺带把标准 `log` 包桥接过来，所以库在 `systray.Logger` 为 nil 时写的行也落在同一个文件里）。写日志时：

- **消息是稳定的模板，变量进字段**：`slog.Info("service started", "pid", pid)`，而不是 `log.Printf("service pid %d", pid)`。前者的每一行都是同一条 `msg`，可以按 `msg=` 检索、按字段过滤；后者每次都是一条新消息，既数不出来也过滤不了。
- **级别按语义**：`Error` 是「这件事失败了」，`Warn` 是「还能继续，但值得知道」（服务没起来、排队的活儿迟了 48 秒、自启动没写成），`Info` 是启动与状态。
- **字段用词固定**：`error`、`path`、`url`、`pid`、`status`、`waited`、`late`、`line`；`windowState` 自己实现了 `LogValue`，于是落成 `state.width=726 state.onTop=false` 这样的分组。

</details>

## 数据与日志

都在 `%LOCALAPPDATA%\deepseek-harness-desktop\`：

| 路径 | 是什么 |
|---|---|
| `window-state.json` | 窗口大小、是否最大化、上次打开的地址（**不含 token**）；位置有意不记 |
| `app.log` | 运行日志，超过 1 MiB 轮转一次 |
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
