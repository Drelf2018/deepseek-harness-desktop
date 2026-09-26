package main

// What a notification is made of: the template, the XML payload it renders, and the PowerShell
// that carries it. These are formats where one character nobody thought about loses the whole
// toast, and a windowsgui build has nowhere to say so.

import (
	"encoding/base64"
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

// between returns what sits between the first open and the next close after it.
func between(s, open, close string) (string, bool) {
	i := strings.Index(s, open)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// 载荷由模板生成，所以这里从渲染结果里把它取回来：取回来的还要是能解析的 XML，两行文字还要与给
// 进去的一模一样——转义、模板、数据三件事一起证明。
func TestToastPayloadRoundTrips(t *testing.T) {
	const (
		title = `A & B <"检查"> it's`
		body  = "it's a trap & 一次审批"
	)
	script, err := renderNotify(title, body)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	payload, ok := between(script, "$xml.LoadXml('", "')")
	if !ok {
		t.Fatalf("the rendered script has no payload:\n%s", script)
	}

	var doc struct {
		Launch string   `xml:"launch,attr"`
		Texts  []string `xml:"visual>binding>text"`
	}
	if err := xml.Unmarshal([]byte(payload), &doc); err != nil {
		t.Fatalf("the payload is not XML: %v\n%s", err, payload)
	}
	if doc.Launch != notificationURL {
		t.Errorf("launch = %q, want %q", doc.Launch, notificationURL)
	}
	// Two lines, and they are the ones the page asked for - which is only true if the escaping
	// was exactly right.
	if len(doc.Texts) != 2 || doc.Texts[0] != title || doc.Texts[1] != body {
		t.Errorf("text = %#v, want %q and %q", doc.Texts, title, body)
	}
	// 这段 XML 会被原样塞进 PowerShell 的单引号字符串里：里面剩一个撇号，那条命令就从中间断
	// 开，而断掉之后的表现是「没有通知」，不是报错。
	if strings.Contains(payload, "'") {
		t.Errorf("the payload carries a single quote, which would end the PowerShell string: %s", payload)
	}
}

// 渲染完的脚本里不能再有模板动作或没填上的值。填漏了不会报错，只会让 PowerShell 收到一句读不通
// 的命令，而外面看到的只是「通知没来」。
func TestToastScriptIsFullyRendered(t *testing.T) {
	script, err := renderNotify("标题", "正文")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(script, "{{") {
		t.Errorf("a template action survived the render:\n%s", script)
	}
	if !strings.Contains(script, "CreateToastNotifier('"+appID+"')") {
		t.Errorf("the app's own AppUserModelID did not reach the script:\n%s", script)
	}
	if !strings.Contains(script, notificationURL) {
		t.Errorf("the launch address did not reach the payload:\n%s", script)
	}
}

// 正文里出现和模板动作一样的字样时，它只是正文：模板走一遍，写出值之后不会回头再解析一遍它。否则
// 提问里写了模板动作，appID 就会连着被塞进它自己的正文。
func TestToastScriptDoesNotRescanThePayload(t *testing.T) {
	const body = "正文里真的写了 {{.Notifier}} 这几个字"
	script, err := renderNotify("标题", body)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(script, body) {
		t.Errorf("the payload was changed on the way in:\n%s", script)
	}
}

// notify.ps1 只有 ASCII，一条通知里唯一可能是中文的是载荷，而载荷是运行时填进去的。
func TestToastScriptStaysASCII(t *testing.T) {
	for i, ch := range notifyPS {
		if ch > 127 {
			t.Fatalf("notify.ps1 has a non-ASCII character at %d: %q", i, ch)
		}
	}
}

// -EncodedCommand 要的是 UTF-16LE 的 base64。字节序反了同样是合法 base64，PowerShell 只会
// 收到一串乱码，所以这里把字节也钉住。
func TestUTF16LEBase64(t *testing.T) {
	const script = "$xml = '确定数据'\r\nÉ"
	data, err := base64.StdEncoding.DecodeString(utf16leBase64(script))
	if err != nil {
		t.Fatalf("not base64: %v", err)
	}
	if len(data)%2 != 0 {
		t.Fatalf("an odd number of bytes cannot be UTF-16: %d", len(data))
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = uint16(data[i*2]) | uint16(data[i*2+1])<<8
	}
	if got := string(utf16.Decode(units)); got != script {
		t.Errorf("decoded = %q, want %q", got, script)
	}
	if data[0] != '$' || data[1] != 0 {
		t.Errorf("the first unit is %#04x, want 0x0024: the encoding has to be little endian", uint16(data[0])|uint16(data[1])<<8)
	}
}

// The script and the binding it calls live in two files, and renaming one of them is silent:
// no toast, no error, nothing in the log. CI only runs node --check, which does not know the
// two names are related; this does.
func TestNotifyScriptAgreesWithTheApp(t *testing.T) {
	if !strings.Contains(notifyJS, "window."+notifyBinding) {
		t.Errorf("js/notify.js does not call window.%s", notifyBinding)
	}
	// 这三个属性是页面的 DOM 与这里唯一说好的东西。
	for _, attr := range []string{"data-approval-key", "data-question-key", "data-plan-review-key"} {
		if !strings.Contains(notifyJS, attr) {
			t.Errorf("js/notify.js does not look for %s", attr)
		}
	}
}
