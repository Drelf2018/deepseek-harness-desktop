//go:build windows

package main

// The pages this app shows when there is nothing of the service's to show.
//
// They are built here and handed to the window as data URLs, which is the one kind of page
// that needs no listener, no file and no second server - so it is still there when the
// service is down, and it cannot answer 401 the way an unauthorised address does.
//
// The window is a browser, and a failure used to leave it on the 401 the service answers
// before it has been authorised: a blank page with a status code in it, and no way to tell
// a service that is still coming up from a port held by someone else's service.
//
// The markup is loading.html, beside this file: HTML belongs in a file, where an editor
// highlights, indents and checks it. Go contributes the two texts and nothing else -
// html/template escapes by context, so nothing here escapes by hand.

import (
	"bytes"
	"embed"
	"encoding/base64"
	"html/template"
	"log/slog"
)

const (
	startingTitle = "正在启动 DSH 服务"
	startingBody  = `服务正在启动，页面就绪后会自动打开。

第一次运行需要下载，可能要等一会儿。
若长时间没有反应，请查看日志：`

	blockedTitle = "无法连接到 DSH 服务"
	blockedBody  = `端口 3080 上已经有一个 DSH 在运行，本次运行拿不到它的访问令牌。

请先退出那个实例，再重新启动本程序。
或者从托盘菜单「设置访问地址」手动指定一个地址。`

	unannouncedTitle = "无法连接到 DSH 服务"
	unannouncedBody  = `服务已经启动，但它没有在预期时间内给出访问链接。

请查看日志确认服务是否正常启动：`
)

//go:embed loading.html
var noticeFS embed.FS

// noticeLayout is the one page layout every notice above is poured into, parsed once at startup.
//
// Must, and not an error carried up to main: the layout is a file in this repository, and the way
// to find out it is broken is to run the program - or the tests, whose binary the panic takes down
// with it. A -H=windowsgui build has no stderr, so a panic here ends the program in silence; that
// is the price of not carrying an error nobody can act on to the top of main.
//
// The file is named rather than globbed on purpose: with a glob, a second layout file would be
// parsed and then quietly ignored, because Execute on a parsed set runs whichever template the
// set was named after - the first file - and nothing would say so.
var noticeLayout = template.Must(template.ParseFS(noticeFS, "loading.html"))

// noticeData is what the layout is filled with.
type noticeData struct {
	Title string
	Body  string
	Path  string
}

// noticePage builds one small self-contained page. The markup is deliberately plain: this is a
// message, not a page anyone styles.
func noticePage(title, body, path string) string {
	page := bytes.NewBufferString("data:text/html;charset=utf-8;base64,")
	enc := base64.NewEncoder(base64.StdEncoding, page)
	err := noticeLayout.Execute(enc, noticeData{Title: title, Body: body, Path: path})
	if err != nil {
		slog.Error("notice page", "error", err)
		return ""
	}
	err = enc.Close()
	if err != nil {
		slog.Error("notice page", "error", err)
		return ""
	}
	return page.String()
}

// startingPage is what the window shows while the service is coming up: an address that
// answers 401 would be a worse answer than one that says what is happening.
func startingPage() string {
	return noticePage(startingTitle, startingBody, logPath())
}

// blockedPage is shown when the port belongs to a service this run did not start.
func blockedPage() string {
	return noticePage(blockedTitle, blockedBody, "")
}

// unannouncedPage is shown when the service started but never printed its link.
func unannouncedPage() string {
	return noticePage(unannouncedTitle, unannouncedBody, logPath())
}
