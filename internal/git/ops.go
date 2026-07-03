package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"multigit/internal/command"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type CommitInfo struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

func CurrentBranch(cwd string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func IsRepo(cwd string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run() == nil
}

func statusHasChanges(cwd string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

func Stash(ctx context.Context, cwd string) (bool, error) {
	hasChanges, err := statusHasChanges(cwd)
	if err != nil {
		return false, err
	}
	if !hasChanges {
		return false, nil
	}
	runtime.EventsEmit(ctx, "log", "[git] 正在储藏本地修改...")
	if err := command.Run(ctx, "git", []string{"stash"}, cwd); err != nil {
		runtime.EventsEmit(ctx, "log", "[git] 储藏失败")
		return false, err
	}
	runtime.EventsEmit(ctx, "log", "[git] 本地修改已储藏")
	return true, nil
}

func StashPop(ctx context.Context, cwd string) {
	cmd := exec.Command("git", "stash", "list")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return
	}

	runtime.EventsEmit(ctx, "log", "[git] 正在恢复储藏...")
	if err := command.Run(ctx, "git", []string{"stash", "pop"}, cwd); err != nil {
		runtime.EventsEmit(ctx, "log", "[git] 恢复储藏失败，请手动执行 git stash pop")
		return
	}
	runtime.EventsEmit(ctx, "log", "[git] 储藏已恢复")
}

func SwitchOrCreate(ctx context.Context, cwd string, branchName string) error {
	localBranches, err := listLocalBranches(cwd)
	if err != nil {
		return err
	}

	for _, b := range localBranches {
		if b == branchName {
			runtime.EventsEmit(ctx, "log", "[git] 切换到已有本地分支: "+branchName)
			return command.Run(ctx, "git", []string{"checkout", branchName}, cwd)
		}
	}

	runtime.EventsEmit(ctx, "log", "[git] 正在拉取远程...")
	if err := command.Run(ctx, "git", []string{"fetch"}, cwd); err != nil {
		return err
	}

	remoteBranches, err := listRemoteBranches(cwd)
	if err != nil {
		return err
	}

	remoteFull := "origin/" + branchName
	for _, b := range remoteBranches {
		if b == remoteFull {
			runtime.EventsEmit(ctx, "log", "[git] 从远程创建并切换分支: "+branchName)
			return command.Run(ctx, "git", []string{"checkout", "-b", branchName, remoteFull}, cwd)
		}
	}

	runtime.EventsEmit(ctx, "log", "[git] 创建新分支: "+branchName)
	return command.Run(ctx, "git", []string{"checkout", "-b", branchName}, cwd)
}

func Pull(ctx context.Context, cwd string, branch string) error {
	runtime.EventsEmit(ctx, "log", "[git] 正在拉取 origin/"+branch+"...")
	return command.Run(ctx, "git", []string{"pull", "origin", branch, "--progress"}, cwd)
}

func CommitAndPush(ctx context.Context, cwd string, message string, branch string) error {
	runtime.EventsEmit(ctx, "log", "[git] 添加文件...")
	args := []string{"add", "package.json"}
	lockPath := filepath.Join(cwd, "package-lock.json")
	if _, err := os.Stat(lockPath); err == nil {
		args = append(args, "package-lock.json")
	}
	if err := command.Run(ctx, "git", args, cwd); err != nil {
		return err
	}

	runtime.EventsEmit(ctx, "log", "[git] 提交: "+message)
	if err := command.Run(ctx, "git", []string{"commit", "-m", message}, cwd); err != nil {
		return err
	}

	runtime.EventsEmit(ctx, "log", "[git] 推送到 origin/"+branch+"...")
	return command.Run(ctx, "git", []string{"push", "origin", branch}, cwd)
}

func ResetHard(ctx context.Context, cwd string, ref string) {
	runtime.EventsEmit(ctx, "log", "[git] 正在重置 --hard "+ref+"...")
	command.Run(ctx, "git", []string{"reset", "--hard", ref}, cwd)
}

func listLocalBranches(cwd string) ([]string, error) {
	cmd := exec.Command("git", "branch")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

func listRemoteBranches(cwd string) ([]string, error) {
	cmd := exec.Command("git", "branch", "-r")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

func BranchExists(cwd string, branch string) (bool, error) {
	localBranches, err := listLocalBranches(cwd)
	if err != nil {
		return false, err
	}
	for _, b := range localBranches {
		if b == branch {
			return true, nil
		}
	}

	remoteBranches, err := listRemoteBranches(cwd)
	if err != nil {
		return false, err
	}
	remote := "origin/" + branch
	for _, b := range remoteBranches {
		if b == remote {
			return true, nil
		}
	}

	return false, nil
}

func Fetch(ctx context.Context, cwd string) error {
	runtime.EventsEmit(ctx, "log", "[git] 正在拉取远程...")
	err := command.Run(ctx, "git", []string{"fetch", "origin"}, cwd)
	if err == nil {
		runtime.EventsEmit(ctx, "log", "[git] 远程拉取完成")
	}
	return err
}

func Log(cwd, ref string, n int) ([]CommitInfo, error) {
	cmd := exec.Command("git", "log", ref, "--reverse", fmt.Sprintf("-n%d", n), "--format=%H%x00%an%x00%ar%x00%s")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) == 4 {
			commits = append(commits, CommitInfo{Hash: parts[0], Author: parts[1], Date: parts[2], Message: parts[3]})
		}
	}
	return commits, nil
}

func IsAncestor(cwd, hash, branch string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", hash, branch)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func CherryPick(ctx context.Context, cwd string, hash string) error {
	return command.Run(ctx, "git", []string{"cherry-pick", hash}, cwd)
}

func CherryPickAbort(ctx context.Context, cwd string) {
	command.Run(ctx, "git", []string{"cherry-pick", "--abort"}, cwd)
}

func PushBranch(ctx context.Context, cwd string, branch string) error {
	runtime.EventsEmit(ctx, "log", "[git] 推送到 origin/"+branch+"...")
	return command.Run(ctx, "git", []string{"push", "origin", branch}, cwd)
}
