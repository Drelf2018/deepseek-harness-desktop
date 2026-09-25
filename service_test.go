package main

// What the service prints, and what is done with it. The token in the announced link is
// minted per process: it is the only address that works, and it is never written down.

import (
	"strings"
	"testing"
)

// A real token is 43 characters of base64url, and a real announcement is the word dsh, the
// profile, and the link.
var token = strings.Repeat("D", 43)

const (
	announce = "dsh web: "
	link     = "http://127.0.0.1:3080/?token="
)

// resetService clears what a previous test left in the writer's buffer.
func resetService() {
	serviceMu.Lock()
	serviceOutput.Reset()
	serviceLink = ""
	serviceMu.Unlock()
}

func TestServiceWriterFindsTheLink(t *testing.T) {
	resetService()
	w := serviceWriter{}
	// 这一行是分成几块到的，管道就是这么送的。在它完整之前什么都不能拿：半个链接就是半个
	// token。
	for _, piece := range []string{
		"[dsh] starting\n" + announce + link + token[:20],
		token[20:],
		"\n",
	} {
		if _, err := w.Write([]byte(piece)); err != nil {
			t.Fatalf("write %q: %v", piece, err)
		}
	}
	if got, want := announcedLink(), link+token; got != want {
		t.Fatalf("announcedLink() = %q, want %q", got, want)
	}
}

func TestServiceWriterIgnoresOtherURLs(t *testing.T) {
	resetService()
	w := serviceWriter{}
	if _, err := w.Write([]byte("listening on http://127.0.0.1:3080/ and http://192.168.1.5:3080/\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := announcedLink(); got != "" {
		t.Fatalf("announcedLink() = %q, want nothing: only the announce line counts", got)
	}
}

func TestServiceWriterTakesTheFirstLink(t *testing.T) {
	resetService()
	w := serviceWriter{}
	// 同一行后面跟着的那个局域网地址是给别的机器用的。
	if _, err := w.Write([]byte(announce + link + "abc (LAN: http://192.168.1.5:3080/?token=abc)\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, want := announcedLink(), link+"abc"; got != want {
		t.Fatalf("announcedLink() = %q, want %q", got, want)
	}
}

func TestStripToken(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{link + token, "http://127.0.0.1:3080/"},
		{"http://127.0.0.1:3080/?foo=1&token=abc", "http://127.0.0.1:3080/?foo=1"},
		{"http://127.0.0.1:3080/", "http://127.0.0.1:3080/"},
		{"about:blank", "about:blank"},
		{"not a url", "not a url"},
	} {
		if got := stripToken(c.in); got != c.want {
			t.Errorf("stripToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
