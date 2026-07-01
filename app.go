package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"gitdesk/internal/config"
	"gitdesk/internal/git"
	"gitdesk/internal/npm"

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
	name, _ := syscall.UTF16PtrFromString("GitDesk")
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

func (a *App) UpdatePackage(dirList []string, packages []string, branch string) (UpdateResult, error) {
	runtime.EventsEmit(a.ctx, "log", "GitDesk 批量依赖更新")
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
