package main

// The pages the app shows when there is nothing of the service's to show. They are built
// here rather than served, so they are still there when the service is not.

import (
	"encoding/base64"
	"strings"
	"testing"
)

// decodeNotice unwraps the data URL into the page it carries, the way the browser does.
func decodeNotice(t *testing.T, page string) string {
	t.Helper()
	const prefix = "data:text/html;charset=utf-8;base64,"
	if !strings.HasPrefix(page, prefix) {
		t.Fatalf("page = %.60q..., want a base64 data URL: it has to survive being handed to a browser with no listener behind it", page)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(page, prefix))
	if err != nil {
		t.Fatalf("decode the page: %v", err)
	}
	return string(raw)
}

func TestNoticePages(t *testing.T) {
	if logPath() == "" {
		t.Fatal("logPath is empty, so the two pages that name it cannot be checked")
	}
	for _, c := range []struct {
		name  string
		page  string
		title string
		// path is whether this page offers to name the log file. The blocked page tells the
		// reader what to do about the other instance instead, which is a copy decision and
		// not this test's business.
		path bool
	}{
		{"starting", startingPage(), startingTitle, true},
		{"blocked", blockedPage(), blockedTitle, false},
		{"unannounced", unannouncedPage(), unannouncedTitle, true},
	} {
		html := decodeNotice(t, c.page)
		if !strings.Contains(html, c.title) {
			t.Errorf("%s page does not carry its title %q", c.name, c.title)
		}
		// 路径不是抄进文字里的一份，而是 logPath() 现给的：这里查的就是它有没有真的到页面上。
		if c.path && !strings.Contains(html, logPath()) {
			t.Errorf("%s page does not name the log file %q:\n%s", c.name, logPath(), html)
		}
		// 有路径的页面要真的装上那个框；没有路径的页面，那个框也不该在。框里的元素是 <code>，
		if c.path && !strings.Contains(html, "<code") {
			t.Errorf("%s page names the log file but carries no box for it:\n%s", c.name, html)
		}
		if !c.path && strings.Contains(html, "<code") {
			t.Errorf("%s page has no path to show, yet carries the box for one:\n%s", c.name, html)
		}
	}
}

func TestNoticePageEscapesItsText(t *testing.T) {
	// 标题、正文、路径三者都过一遍引擎：路径里同样会有 < 和 &（用户名里就可能有）。
	html := decodeNotice(t, noticePage("T & T", "<not a tag>", `C:\a & b\log`))
	for _, want := range []string{"T &amp; T", "&lt;not a tag&gt;", `C:\a &amp; b\log`} {
		if !strings.Contains(html, want) {
			t.Fatalf("text handed to noticePage was not escaped as %q:\n%s", want, html)
		}
	}
}
