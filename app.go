package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sort"
	"syscall"
	"time"
	"unsafe"
"multigit/internal/config"

	"multigit/internal/git"

	"multigit/internal/npm"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed appicon.ico
var iconICO []byte

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	procCreateIconFromResourceEx   = user32.NewProc("CreateIconFromResourceEx")
	procSendMessageW               = user32.NewProc("SendMessageW")
	procFindWindowW                = user32.NewProc("FindWindowW")
)

type icoEntry struct {
	width  int
	height int
	data   []byte
}

func findICOIconResource(icoData []byte, targetSize int) *icoEntry {
	if len(icoData) < 22 {
		return nil
	}
	count := int(icoData[4]) | int(icoData[5])<<8
	if count == 0 {
		return nil
	}
	offset := 6
	for i := 0; i < count; i++ {
		if offset+16 > len(icoData) {
			break
		}
		w := int(icoData[offset])
		h := int(icoData[offset+1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		size := int(icoData[offset+8]) | int(icoData[offset+9])<<8 | int(icoData[offset+10])<<16 | int(icoData[offset+11])<<24
		dataOff := int(icoData[offset+12]) | int(icoData[offset+13])<<8 | int(icoData[offset+14])<<16 | int(icoData[offset+15])<<24
		if dataOff+size <= len(icoData) && w == targetSize && h == targetSize {
			return &icoEntry{width: w, height: h, data: icoData[dataOff : dataOff+size]}
		}
		offset += 16
	}
	return nil
}

func (a *App) getHWND() uintptr {
	name, _ := syscall.UTF16PtrFromString("MultiGit")
	hwnd, _, _ := procFindWindowW.Call(0, 0, uintptr(unsafe.Pointer(name)), 0)
	return hwnd
}

func (a *App) setWindowIcon() {
	time.Sleep(300 * time.Millisecond)
	hwnd := a.getHWND()
	if hwnd == 0 {
		time.Sleep(500 * time.Millisecond)
		hwnd = a.getHWND()
	}
	if hwnd == 0 {
		return
	}

	res := findICOIconResource(iconICO, 32)
	if res == nil {
		res = findICOIconResource(iconICO, 16)
	}
	if res == nil {
		return
	}

	hIcon, _, _ := procCreateIconFromResourceEx.Call(
		uintptr(unsafe.Pointer(&res.data[0])),
		uintptr(len(res.data)),
		1,
		0x00030000,
		uintptr(res.width),
		uintptr(res.height),
		0,
	)
	if hIcon != 0 {
		const WM_SETICON = 0x0080
		const ICON_SMALL = 0
		const ICON_BIG = 1
		procSendMessageW.Call(hwnd, WM_SETICON, ICON_SMALL, hIcon)
		procSendMessageW.Call(hwnd, WM_SETICON, ICON_BIG, hIcon)
	}
}

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.setWindowIcon()
}

func (a *App) LoadConfig() (config.Config, error) {
	return config.Load()
}

func (a *App) SaveConfig(cfg config.Config) error {
	return config.Save(cfg)
}

func (a *App) SelectDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择项目根目录",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

type ProjectItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (a *App) ListProjects(rootPath string) ([]ProjectItem, error) {
	if rootPath == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	var projects []ProjectItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(rootPath, entry.Name())
		if !git.IsRepo(dirPath) {
			continue
		}
		projects = append(projects, ProjectItem{
			Name: entry.Name(),
			Path: filepath.ToSlash(dirPath),
		})
	}

	return projects, nil
}

type UpdateResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type ProjectCommits struct {
	ProjectName string          `json:"project_name"`
	ProjectPath string          `json:"project_path"`
	Commits     []git.CommitInfo `json:"commits"`
}

type SearchResult struct {
	Results []ProjectCommits `json:"results"`
	HasMore bool             `json:"hasMore"`
}

func (a *App) UpdatePackage(dirList []string, packages []string, branch string) (UpdateResult, error) {
	runtime.EventsEmit(a.ctx, "log", "MultiGit 批量依赖更新")
	runtime.EventsEmit(a.ctx, "log", "========================================")

	cfg, err := config.Load()
	if err != nil {
		return UpdateResult{OK: false, Message: "加载配置失败: " + err.Error()}, nil
	}

	runtime.EventsEmit(a.ctx, "log", "正在验证依赖包是否存在...")
	var allVerified = true
	for _, pkg := range packages {
		ok, err := npm.VerifyPackage(a.ctx, pkg, cfg.Registry)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("  %s - 验证失败", pkg))
			allVerified = false
		} else if ok {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("  %s - 验证通过", pkg))
		} else {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("  %s - 未找到", pkg))
			allVerified = false
		}
	}

	if !allVerified {
		return UpdateResult{OK: false, Message: "依赖包验证失败"}, nil
	}
	runtime.EventsEmit(a.ctx, "log", "所有依赖包验证通过")
	runtime.EventsEmit(a.ctx, "log", "")

	successCount := 0
	failCount := 0

	for _, cwd := range dirList {
		projectName := filepath.Base(cwd)
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("项目: %s (%s)", projectName, cwd))

		oldBranch, err := git.CurrentBranch(cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("获取当前分支失败: %s", err.Error()))
			failCount++
			continue
		}

		isStash, err := git.Stash(a.ctx, cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("储藏失败: %s", err.Error()))
			failCount++
			continue
		}

		committed := false
		err = func() error {
			if err := git.SwitchOrCreate(a.ctx, cwd, branch); err != nil {
				return fmt.Errorf("切换分支失败: %w", err)
			}

			currentBranch, err := git.CurrentBranch(cwd)
			if err != nil {
				return fmt.Errorf("获取当前分支失败: %w", err)
			}
			if currentBranch != branch {
				return fmt.Errorf("分支切换失败，期望 %s 实际 %s", branch, currentBranch)
			}

			if err := git.Pull(a.ctx, cwd, branch); err != nil {
				return fmt.Errorf("拉取失败: %w", err)
			}

			shouldBump := contains(cfg.AutoBumpProjects, projectName)
			updated, err := npm.ApplyUpdates(cwd, packages, shouldBump, "devDependencies")
			if err != nil {
				return fmt.Errorf("更新依赖失败: %w", err)
			}

			if len(updated) > 0 {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("已更新依赖: %s", strings.Join(updated, ", ")))
			}
			if shouldBump && len(updated) == 0 {
				runtime.EventsEmit(a.ctx, "log", "更新 package.json 版本号")
			}

			if len(updated) > 0 || shouldBump {
				if err := npm.Install(a.ctx, cwd, cfg.Registry); err != nil {
					return fmt.Errorf("npm install 失败: %w", err)
				}

				commitMsg := "chore: 更新依赖 " + strings.Join(updated, ", ")
				if len(updated) == 0 {
					commitMsg = "chore: 更新版本号"
				}

				if err := git.CommitAndPush(a.ctx, cwd, commitMsg, branch); err != nil {
					return fmt.Errorf("提交失败: %w", err)
				}
				committed = true
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("完成: %s", projectName))
			} else {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("跳过: %s (无需更新)", projectName))
			}
			return nil
		}()

		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("失败: %s - %s", projectName, err.Error()))
			failCount++
			if committed {
				git.ResetHard(a.ctx, cwd, "HEAD~1")
				git.ResetHard(a.ctx, cwd, "HEAD")
			} else {
				git.ResetHard(a.ctx, cwd, "HEAD")
			}
		} else {
			successCount++
		}

		if err := git.SwitchOrCreate(a.ctx, cwd, oldBranch); err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("切回原分支失败: %s", err.Error()))
		}
		if isStash {
			git.StashPop(a.ctx, cwd)
		}
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", "")
	}

	msg := fmt.Sprintf("完成: %d 成功, %d 失败", successCount, failCount)
	runtime.EventsEmit(a.ctx, "log", msg)
	runtime.EventsEmit(a.ctx, "log", "========================================")

	return UpdateResult{OK: true, Message: msg}, nil
}

func (a *App) GetRecentCommits(dirList []string, branch string, count int) []ProjectCommits {
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		result []ProjectCommits
	)

	for _, cwd := range dirList {
		wg.Add(1)
		cwd := cwd
		go func() {
			defer wg.Done()

			projectName := filepath.Base(cwd)
			if !git.IsRepo(cwd) {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("跳过: %s (不是 git 仓库)", projectName))
				return
			}

			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("获取 %s 的提交记录...", projectName))

			if err := git.Fetch(a.ctx, cwd); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("获取 %s 失败: %s", projectName, err.Error()))
				return
			}

			remoteRef := "origin/" + branch
			commits, err := git.Log(cwd, remoteRef, count, 0)
			if err != nil {
				commits, err = git.Log(cwd, branch, count, 0)
				if err != nil {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("获取 %s 提交记录失败: %s", projectName, err.Error()))
					return
				}
			}

			mu.Lock()
			result = append(result, ProjectCommits{
				ProjectName: projectName,
				ProjectPath: cwd,
				Commits:     commits,
			})
			mu.Unlock()
		}()
	}

	wg.Wait()
	return result
}

func (a *App) ListProjectBranches(dirList []string) map[string]interface{} {
	seen := map[string]int{}
	total := len(dirList)

	for _, cwd := range dirList {
		if !git.IsRepo(cwd) {
			continue
		}
		branches, err := git.ListBranches(cwd)
		if err != nil {
			continue
		}
		for _, b := range branches {
			seen[b]++
		}
	}

	var allBranches []string
	for b := range seen {
		allBranches = append(allBranches, b)
	}
	sort.Strings(allBranches)

	branchCounts := make(map[string]int)
	for _, b := range allBranches {
		branchCounts[b] = seen[b]
	}

	return map[string]interface{}{
		"allBranches":   allBranches,
		"branchCounts":  branchCounts,
		"totalProjects": total,
	}
}

func (a *App) GetRecentCommitsPage(dirList []string, branch string, count, skip int) []ProjectCommits {
	var result []ProjectCommits

	for _, cwd := range dirList {
		projectName := filepath.Base(cwd)
		if !git.IsRepo(cwd) {
			continue
		}

		remoteRef := "origin/" + branch
		commits, err := git.Log(cwd, remoteRef, count, skip)
		if err != nil {
			commits, err = git.Log(cwd, branch, count, skip)
			if err != nil {
				continue
			}
		}

		result = append(result, ProjectCommits{
			ProjectName: projectName,
			ProjectPath: cwd,
			Commits:     commits,
		})
	}

	return result
}

func (a *App) SearchHistoryCommitsPage(dirList []string, branch, term string, pageSize, skip int) SearchResult {
	result := make([]ProjectCommits, 0)
	allHasMore := false

	for _, cwd := range dirList {
		projectName := filepath.Base(cwd)
		if !git.IsRepo(cwd) {
			continue
		}

		remoteRef := "origin/" + branch
		commits, hasMore, err := git.SearchCommitsPage(cwd, remoteRef, term, pageSize, skip)
		if err != nil {
			commits, hasMore, err = git.SearchCommitsPage(cwd, branch, term, pageSize, skip)
			if err != nil {
				continue
			}
		}

		if hasMore {
			allHasMore = true
		}
		if len(commits) > 0 {
			result = append(result, ProjectCommits{
				ProjectName: projectName,
				ProjectPath: cwd,
				Commits:     commits,
			})
		}
	}

	return SearchResult{Results: result, HasMore: allHasMore}
}

func (a *App) GetCommitFiles(projectPath, hash string) []git.FileChange {
	files, err := git.GetCommitFiles(projectPath, hash)
	if err != nil {
		return nil
	}
	return files
}

func (a *App) GetCommitFileDiff(projectPath, hash, filePath string) string {
	diff, err := git.GetCommitFileDiff(projectPath, hash, filePath)
	if err != nil {
		return ""
	}
	return diff
}

func (a *App) GetCommitFileDiffSideBySide(projectPath, hash, filePath string) map[string]interface{} {
	hunks, tooLarge, err := git.GetCommitFileDiffSideBySide(projectPath, hash, filePath)
	if err != nil {
		return map[string]interface{}{"hunks": nil, "tooLarge": false}
	}
	return map[string]interface{}{"hunks": hunks, "tooLarge": tooLarge}
}

func (a *App) ForceCheckoutBranch(dirList []string, sourceBranch string, targetBranch string) UpdateResult {
	runtime.EventsEmit(a.ctx, "log", "MultiGit 强制检出")
	runtime.EventsEmit(a.ctx, "log", "========================================")
	runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("源分支: %s  目标分支: %s", sourceBranch, targetBranch))
	runtime.EventsEmit(a.ctx, "log", "")

	successCount := 0
	failCount := 0

	for _, cwd := range dirList {
		projectName := filepath.Base(cwd)
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("项目: %s (%s)", projectName, cwd))

		oldBranch, err := git.CurrentBranch(cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("获取当前分支失败: %s", err.Error()))
			failCount++
			continue
		}

		isStash, err := git.Stash(a.ctx, cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("储藏失败: %s", err.Error()))
			failCount++
			continue
		}

		projectFailed := false
		func() {
			if err := git.SwitchOrCreate(a.ctx, cwd, sourceBranch); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("切换到源分支失败: %s", err.Error()))
				projectFailed = true
				return
			}
			if err := git.Pull(a.ctx, cwd, sourceBranch); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("拉取源分支失败: %s", err.Error()))
				projectFailed = true
				return
			}

			backupName, err := git.BackupBranch(a.ctx, cwd, targetBranch)
			if err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("备份目标分支失败: %s", err.Error()))
			} else if backupName != "" {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("已创建备份分支: %s", backupName))
			}

			if err := git.ForceCheckoutTo(a.ctx, cwd, targetBranch, sourceBranch); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("强制检出目标分支失败: %s", err.Error()))
				projectFailed = true
				return
			}

			if err := git.ForcePush(a.ctx, cwd, targetBranch); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("强制推送失败: %s", err.Error()))
				projectFailed = true
				return
			}

			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("完成: %s", projectName))
		}()

		if projectFailed {
			failCount++
		} else {
			successCount++
		}

		if err := git.SwitchOrCreate(a.ctx, cwd, oldBranch); err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("切回原分支失败: %s", err.Error()))
		}
		if isStash {
			git.StashPop(a.ctx, cwd)
		}
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", "")
	}

	msg := fmt.Sprintf("完成: %d 成功, %d 失败", successCount, failCount)
	runtime.EventsEmit(a.ctx, "log", msg)
	runtime.EventsEmit(a.ctx, "log", "========================================")

	return UpdateResult{OK: successCount > 0 || failCount == 0, Message: msg}
}

func (a *App) DeleteBackupBranches(dirList []string) UpdateResult {
	runtime.EventsEmit(a.ctx, "log", "MultiGit 清理备份分支")
	runtime.EventsEmit(a.ctx, "log", "========================================")

	totalDeleted := 0
	failCount := 0

	for _, cwd := range dirList {
		projectName := filepath.Base(cwd)
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("项目: %s (%s)", projectName, cwd))

		deleted, err := git.DeleteBackupBranches(a.ctx, cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("清理失败: %s", err.Error()))
			failCount++
			continue
		}
		if len(deleted) == 0 {
			runtime.EventsEmit(a.ctx, "log", "没有找到备份分支")
		} else {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("已删除 %d 个备份分支: %s", len(deleted), strings.Join(deleted, ", ")))
			totalDeleted += len(deleted)
		}
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", "")
	}

	msg := fmt.Sprintf("完成: 共删除 %d 个备份分支, %d 个项目失败", totalDeleted, failCount)
	runtime.EventsEmit(a.ctx, "log", msg)
	runtime.EventsEmit(a.ctx, "log", "========================================")

	return UpdateResult{OK: true, Message: msg}
}

func (a *App) RestoreBackupBranches(dirList []string) UpdateResult {
	runtime.EventsEmit(a.ctx, "log", "MultiGit 还原备份分支")
	runtime.EventsEmit(a.ctx, "log", "========================================")

	totalRestored := 0
	failCount := 0

	for _, cwd := range dirList {
		projectName := filepath.Base(cwd)
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("项目: %s (%s)", projectName, cwd))

		oldBranch, err := git.CurrentBranch(cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("获取当前分支失败: %s", err.Error()))
			failCount++
			continue
		}

		isStash, err := git.Stash(a.ctx, cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("储藏失败: %s", err.Error()))
			failCount++
			continue
		}

		restored, err := git.RestoreBackupBranches(a.ctx, cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("还原失败: %s", err.Error()))
			failCount++
		} else if len(restored) == 0 {
			runtime.EventsEmit(a.ctx, "log", "没有找到备份分支")
		} else {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("已还原 %d 个分支: %s", len(restored), strings.Join(restored, ", ")))
			totalRestored += len(restored)
		}

		if err := git.SwitchOrCreate(a.ctx, cwd, oldBranch); err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("切回原分支失败: %s", err.Error()))
		}
		if isStash {
			git.StashPop(a.ctx, cwd)
		}
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", "")
	}

	msg := fmt.Sprintf("完成: 共还原 %d 个分支, %d 个项目失败", totalRestored, failCount)
	runtime.EventsEmit(a.ctx, "log", msg)
	runtime.EventsEmit(a.ctx, "log", "========================================")

	return UpdateResult{OK: true, Message: msg}
}

func (a *App) CherryPickCommits(dirList []string, sourceBranch string, selectedCommits map[string][]string, destBranch string) UpdateResult {
	runtime.EventsEmit(a.ctx, "log", "MultiGit Cherry-Pick")
	runtime.EventsEmit(a.ctx, "log", "========================================")

	successCount := 0
	failCount := 0

	for _, cwd := range dirList {
		hashes, ok := selectedCommits[cwd]
		if !ok || len(hashes) == 0 {
			continue
		}

		projectName := filepath.Base(cwd)
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("项目: %s (%s)", projectName, cwd))

		oldBranch, err := git.CurrentBranch(cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("获取当前分支失败: %s", err.Error()))
			failCount++
			continue
		}

		isStash, err := git.Stash(a.ctx, cwd)
		if err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("储藏失败: %s", err.Error()))
			failCount++
			continue
		}

		projectFailed := false
		func() {
			if err := git.SwitchOrCreate(a.ctx, cwd, sourceBranch); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("切换到源分支失败: %s", err.Error()))
				projectFailed = true
				return
			}
			if err := git.Pull(a.ctx, cwd, sourceBranch); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("拉取源分支失败: %s", err.Error()))
				projectFailed = true
				return
			}

			if sourceBranch != destBranch {
				exists, err := git.BranchExists(cwd, destBranch)
				if err != nil {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("检查目标分支失败: %s", err.Error()))
					projectFailed = true
					return
				}
				if !exists {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("目标分支 %s 不存在，已跳过", destBranch))
					projectFailed = true
					return
				}

				if err := git.SwitchOrCreate(a.ctx, cwd, destBranch); err != nil {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("切换到目标分支失败: %s", err.Error()))
					projectFailed = true
					return
				}
				if err := git.Pull(a.ctx, cwd, destBranch); err != nil {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("拉取目标分支失败: %s", err.Error()))
					projectFailed = true
					return
				}
			}

			cherryFailed := false
			for _, hash := range hashes {
				exists, err := git.IsAncestor(cwd, hash, "HEAD")
				if err != nil {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("检查 commit %s 失败: %s", hash, err.Error()))
					cherryFailed = true
					break
				}
				if exists {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("跳过: commit %s 已存在于目标分支", hash[:8]))
					continue
				}

				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("cherry-pick %s...", hash[:8]))
				if err := git.CherryPick(a.ctx, cwd, hash); err != nil {
					runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("commit %s 产生冲突，已中止", hash[:8]))
					git.CherryPickAbort(a.ctx, cwd)
					cherryFailed = true
					break
				}
			}

			if cherryFailed {
				projectFailed = true
				return
			}

			if err := git.PushBranch(a.ctx, cwd, destBranch); err != nil {
				runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("推送失败: %s", err.Error()))
				projectFailed = true
				return
			}

			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("完成: %s", projectName))
		}()

		if projectFailed {
			failCount++
		} else {
			successCount++
		}

		if err := git.SwitchOrCreate(a.ctx, cwd, oldBranch); err != nil {
			runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("切回原分支失败: %s", err.Error()))
		}
		if isStash {
			git.StashPop(a.ctx, cwd)
		}
		runtime.EventsEmit(a.ctx, "log", "----------------------------------------")
		runtime.EventsEmit(a.ctx, "log", "")
	}

	msg := fmt.Sprintf("完成: %d 成功, %d 失败", successCount, failCount)
	runtime.EventsEmit(a.ctx, "log", msg)
	runtime.EventsEmit(a.ctx, "log", "========================================")

	return UpdateResult{OK: true, Message: msg}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

type UpdateInfo struct {
	HasUpdate   bool   `json:"has_update"`
	Latest      string `json:"latest"`
	Current     string `json:"current"`
	DownloadURL string `json:"download_url"`
}

func (a *App) CheckUpdate() UpdateInfo {
	result := UpdateInfo{Current: version}

	cfg, err := config.Load()
	if err != nil || cfg.UpdateRepo == "" {
		return result
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", cfg.UpdateRepo)
	req, err := http.NewRequestWithContext(a.ctx, "GET", url, nil)
	if err != nil {
		return result
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return result
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	if !isNewer(latest, version) {
		return result
	}

	result.HasUpdate = true
	result.Latest = release.TagName
	if len(release.Assets) > 0 {
		result.DownloadURL = release.Assets[0].BrowserDownloadURL
	} else {
		result.DownloadURL = release.HTMLURL
	}
	return result
}

func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

func isNewer(latest, current string) bool {
	if current == "dev" {
		return false
	}
	lp := parseVersion(latest)
	cp := parseVersion(current)
	for i := 0; i < 3; i++ {
		if lp[i] > cp[i] {
			return true
		}
		if lp[i] < cp[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	var parts [3]int
	for i, s := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(s)
		if i < 3 {
			parts[i] = n
		}
	}
	return parts
}
