package main

// The resource icon, checked without writing anything: the same drawing internal/genicon packs,
// read back for its structure. The resource itself is written at release time, in a Linux
// container where this package cannot even be compiled. The window's own icons are a different
// thing (window.go).

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/akavel/rsrc/rsrc"
	ico "github.com/biessek/golang-ico"

	"github.com/Drelf2018/deepseek-harness-desktop/internal/artwork"
)

// sysoName is the name the toolchain looks for: any .syso beside the package's Go files is
// linked into it, without being mentioned anywhere.
const sysoName = "rsrc.syso"

func TestIconSyso(t *testing.T) {
	blob, err := artwork.MultiSizeICO(artwork.Sizes)
	if err != nil {
		t.Fatalf("build a multi-size .ico: %v", err)
	}
	// 目录是手写出来的，所以按 Windows 读它的方式读回来验证：要了几个尺寸就有几张图，每张
	// 都是那个尺寸。
	decoded, err := ico.DecodeAll(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("read the .ico back: %v", err)
	}
	if len(decoded) != len(artwork.Sizes) {
		t.Fatalf("the .ico holds %d images, wanted %d", len(decoded), len(artwork.Sizes))
	}
	for i, img := range decoded {
		if b := img.Bounds(); b.Dx() != artwork.Sizes[i] || b.Dy() != artwork.Sizes[i] {
			t.Fatalf("image %d is %dx%d, wanted %d", i, b.Dx(), b.Dy(), artwork.Sizes[i])
		}
	}

	// 打包那一步也走一遍，只是落在临时目录里：rsrc 只认磁盘上的文件，而这里没有理由碰仓库。
	// 架构用运行测试的这台机器的：只有生成器需要把目标架构说清楚（容器里 runtime.GOARCH 是
	// linux/amd64，而不是要发布的那一个）。
	dir := t.TempDir()
	icoPath := filepath.Join(dir, "icon.ico")
	if err := os.WriteFile(icoPath, blob, 0o644); err != nil {
		t.Fatalf("write %s: %v", icoPath, err)
	}
	// 第二个参数是清单，给空串就是没有：这个可执行文件只拿到图标，别的什么都不加。
	out := filepath.Join(dir, sysoName)
	if err := rsrc.Embed(out, runtime.GOARCH, "", icoPath); err != nil {
		t.Fatalf("embed icon in %s: %v", sysoName, err)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read back %s: %v", sysoName, err)
	}
	if len(written) == 0 {
		t.Fatalf("%s is empty", sysoName)
	}
	t.Logf("%s: %d bytes for %s, from %d sizes of the embedded icon", sysoName, len(written), runtime.GOARCH, len(artwork.Sizes))
}
