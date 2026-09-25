package main

// 单实例那套机制里唯一能被安静地测的部分：重启靠的是「接替的那一份一直问到名字空出来为止」，
// 而这个循环可以拿一个自己的名字来试——碰应用那个 mutex 就会把正在跑的实例叫到前面来，
// 那是对用户的打扰，测试不该做。

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestTakeInstanceMutexWaits(t *testing.T) {
	name := appID + "_TestInstanceMutex"

	first, ok := takeInstanceMutex(name, 0)
	if !ok || first == 0 {
		t.Fatalf("头一份没有拿到这个名字：ok=%v handle=%v", ok, first)
	}
	released := false
	defer func() {
		if !released {
			windows.CloseHandle(first)
		}
	}()

	// 名字被别人拿着：等满期限也拿不到。
	start := time.Now()
	if handle, ok := takeInstanceMutex(name, 300*time.Millisecond); ok {
		windows.CloseHandle(handle)
		t.Fatal("别人拿着名字的时候，第二份也拿到了")
	}
	if waited := time.Since(start); waited < 300*time.Millisecond {
		t.Errorf("只等了 %v，应当一直等到期限", waited)
	}

	// 前一份放手之后，下一份立刻就拿到——重启时发生的就是这件事。
	windows.CloseHandle(first)
	released = true
	second, ok := takeInstanceMutex(name, 2*time.Second)
	if !ok || second == 0 {
		t.Fatal("前一份放手之后，接替的那一份仍然拿不到")
	}
	windows.CloseHandle(second)
}

// 接替的那一份不能用 HideWindow 起。GUI 进程的第一次 ShowWindow 会忽略自己的参数、改用
// STARTUPINFO 里的值，而 Go 把 HideWindow 变成 STARTF_USESHOWWINDOW + SW_HIDE——那样重启出来
// 的那一份会把窗口一直藏着，看起来像什么都没启动。这条测试盯住的就是这个决定。
func TestSuccessorIsNotStartedHidden(t *testing.T) {
	cmd, err := successorCommand()
	if err != nil {
		t.Fatalf("拼不出接替命令：%v", err)
	}
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.HideWindow {
		t.Error("接替的那一份被用 HideWindow 起来了：它的窗口会一直不画出来")
	}
	if len(cmd.Args) < 2 || cmd.Args[1] != restartFlag {
		t.Errorf("接替命令是 %q，第一个参数应当是 %q", cmd.Args, restartFlag)
	}
}

// 重启要把影响行为的参数原样传下去：丢掉 -no-service，接替的那一份就会去启动一个服务，
// 而这一份本来是被要求不许碰服务的。
func TestSuccessorArgs(t *testing.T) {
	wasNoService := noService
	t.Cleanup(func() { noService = wasNoService })

	noService = false
	if got := successorArgs(); len(got) != 1 || got[0] != restartFlag {
		t.Errorf("普通启动的接替参数是 %q，应当只有 %q", got, restartFlag)
	}

	noService = true
	got := successorArgs()
	if len(got) != 2 || got[0] != restartFlag || got[1] != "-no-service" {
		t.Errorf("-no-service 那一份的接替参数是 %q，应当带上 -no-service", got)
	}
}
