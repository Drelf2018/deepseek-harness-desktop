//go:build windows

package main

// The registry is a file dsh writes and this program only reads, so what is checked here is
// that the reading survives the shapes that file can legitimately have - a missing one, JSON
// that is not this document, a record with no name or no path - and that the order the sidebar
// shows is the order the menu gets.

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRegistry puts a registry document under a temporary home and answers with that home.
func writeRegistry(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "storages")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建目录：%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workspace.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("写 registry：%v", err)
	}
	return home
}

// The document has one record in the map that no order lists, one id the map does not have, and
// one record with no name: all three are dropped, and what is left keeps the listed order.
func TestReadWorkspaces(t *testing.T) {
	home := writeRegistry(t, `{
  "unit": {"name": "workspace", "version": 2},
  "global": {"initialized": true, "workspaceIds": ["b", "gone", "a", "blank"]},
  "tables": {"workspaces": {
    "a": {"title": "alpha", "path": "C:\\one", "sessionIds": ["session-1"]},
    "b": {"title": "beta", "path": "C:\\two", "createdAt": "2026-09-09T15:02:01.268Z"},
    "blank": {"title": "", "path": "C:\\three"}
  }}
}`)
	got := readWorkspaces(home)
	want := []workspace{
		{Title: "beta", Path: `C:\two`},
		{Title: "alpha", Path: `C:\one`},
	}
	if len(got) != len(want) {
		t.Fatalf("读到 %d 个工作区 %+v，想要 %d 个", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个是 %+v，想要 %+v", i, got[i], want[i])
		}
	}
}

func TestReadWorkspacesWithoutARegistry(t *testing.T) {
	if got := readWorkspaces(t.TempDir()); got != nil {
		t.Errorf("没有文件时读到 %+v，想要 nil", got)
	}
}

func TestReadWorkspacesOnBrokenJSON(t *testing.T) {
	home := writeRegistry(t, "{ 这不是一份 registry }")
	if got := readWorkspaces(home); got != nil {
		t.Errorf("坏文件读到 %+v，想要 nil", got)
	}
}

// dshHome 与 @deepseek-ai/dsh-home-paths 的三级规则一致：$DSH_HOME 非空则用它（并展开 ~），
// 否则 ~/.dsh；空白的 $DSH_HOME 当作没设置——那边特意这么做，免得一个空变量把家定到当前目录。
func TestDshHome(t *testing.T) {
	user, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("找用户目录：%v", err)
	}

	t.Setenv(dshHomeEnv, "   ")
	got, err := dshHome()
	if err != nil {
		t.Fatalf("找家：%v", err)
	}
	if want := filepath.Join(user, ".dsh"); got != want {
		t.Errorf("空白 $DSH_HOME 得到 %q，想要 %q", got, want)
	}

	t.Setenv(dshHomeEnv, `~\probe-home`)
	got, err = dshHome()
	if err != nil {
		t.Fatalf("找家：%v", err)
	}
	if want := filepath.Join(user, "probe-home"); got != want {
		t.Errorf("$DSH_HOME 得到 %q，想要 %q", got, want)
	}
}
