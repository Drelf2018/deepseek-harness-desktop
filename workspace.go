//go:build windows

package main

// The Workspaces the harness knows about, and where the harness keeps them.
//
// They are read, not asked for. dsh keeps one root for all of its user data, and the Workspace
// registry is a file under it - the same list the web sidebar draws. The tray only ever wants a
// name and a path, so those are what this reads; the rest of the document is left alone.

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// dshHomeEnv is the variable @deepseek-ai/dsh-home-paths reads before falling back to ~/.dsh.
const dshHomeEnv = "DSH_HOME"

// workspace is one entry of that registry: what the sidebar calls it, and where it points.
type workspace struct {
	Title string
	Path  string
}

// dshHome is the root the harness resolves, by the rules its own dsh-home-paths package
// documents: an explicit configured path (which nothing outside the harness can see), then
// $DSH_HOME, then ~/.dsh.
//
// A blank or whitespace-only $DSH_HOME counts as unset there, and is treated as unset here for
// the same reason: a stray empty variable must not move the home to the current directory.
func dshHome() (string, error) {
	if env := strings.TrimSpace(os.Getenv(dshHomeEnv)); env != "" {
		return filepath.Abs(expandHome(env))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".dsh"), nil
}

// expandHome mirrors the expansion the harness does, which knows exactly three forms: "~",
// "~/..." and the Windows spelling of the same. A named user such as ~alice, and a variable
// written into the path, pass through untouched there and here.
func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// workspaceFile is the part of the registry document this program reads: the order the sidebar
// shows, and one record per Workspace.
//
// Everything else in the file - archived sessions, each Workspace's session list, the
// timestamps - is deliberately not modelled, so that a field added over there cannot break the
// reading over here.
type workspaceFile struct {
	Global struct {
		WorkspaceIDs []string `json:"workspaceIds"`
	} `json:"global"`
	Tables struct {
		Workspaces map[string]struct {
			Title string `json:"title"`
			Path  string `json:"path"`
		} `json:"workspaces"`
	} `json:"tables"`
}

// readWorkspaces reads the registry under home, in the order the sidebar draws it.
//
// A missing file, JSON that is not this document, and a registry that holds nothing all end
// the same way - no Workspaces - because the caller has nothing to show either way, and three
// error paths would only be three ways to say one thing. A record without a name, or without a
// path, is dropped: a menu item that opens nothing is worse than one that is not there.
func readWorkspaces(home string) []workspace {
	path := filepath.Join(home, "storages", "workspace.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("workspaces: cannot read the registry", "path", path, "error", err)
		}
		return nil
	}
	var file workspaceFile
	if err := json.Unmarshal(data, &file); err != nil {
		slog.Warn("workspaces: cannot parse the registry", "path", path, "error", err)
		return nil
	}
	// 顺序取自 workspaceIds 而不是那张 map：后者没有顺序，而顺序正是侧栏的样子。
	list := make([]workspace, 0, len(file.Global.WorkspaceIDs))
	for _, id := range file.Global.WorkspaceIDs {
		record, ok := file.Tables.Workspaces[id]
		if !ok || record.Title == "" || record.Path == "" {
			slog.Warn("workspaces: skipping an incomplete record", "id", id)
			continue
		}
		list = append(list, workspace{Title: record.Title, Path: record.Path})
	}
	if len(list) == 0 {
		slog.Info("workspaces: nothing to show", "path", path)
	}
	return list
}

// workspaces is the registry of the home this run resolved.
func workspaces() []workspace {
	home, err := dshHome()
	if err != nil {
		slog.Warn("workspaces: cannot find the harness home", "error", err)
		return nil
	}
	return readWorkspaces(home)
}
