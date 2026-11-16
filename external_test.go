package main

// 交给 shell 的地址只允许那几种协议——这条守的是「什么能开」。至于「什么时候问」，在
// js/openExternal.js 那边；两个文件里的名字对不上是静默的，所以顺带也守一下。

import (
	"strings"
	"testing"
)

func TestExternalAddressOpensOnlySafeSchemes(t *testing.T) {
	for _, c := range []struct {
		in string
		ok bool
	}{
		{"https://example.com/a?b=1", true},
		{"http://127.0.0.1:3080/", true},
		{"mailto:someone@example.com", true},
		{"javascript:alert(1)", false},
		{"file:///C:/Windows/System32/cmd.exe", false},
		{"ms-settings:", false},
		{"data:text/html,hi", false},
		{"http://", false},
		{"not a url", false},
		{"", false},
	} {
		got, ok := externalAddress(c.in)
		if ok != c.ok {
			t.Errorf("externalAddress(%q) ok = %v, want %v", c.in, ok, c.ok)
		}
		if ok && got == "" {
			t.Errorf("externalAddress(%q) answered with an empty address", c.in)
		}
	}
}

func TestOpenExternalScriptAgreesWithTheApp(t *testing.T) {
	if !strings.Contains(openExternalJS, "window."+openExternalBinding) {
		t.Errorf("js/openExternal.js does not call window.%s", openExternalBinding)
	}
}
