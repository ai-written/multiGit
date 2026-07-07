package command

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"syscall"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const CREATE_NO_WINDOW = 0x08000000

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func execCommand(name string, args []string, cwd string) *exec.Cmd {
	cwd = filepath.FromSlash(cwd)
	if cwd != "" && strings.HasPrefix(strings.ToLower(cwd), `\\wsl`) {
		parts := strings.SplitN(cwd, `\`, 5)
		if len(parts) >= 4 {
			distro := parts[3]
			wslPath := "/" + strings.ReplaceAll(parts[4], `\`, "/")
			pieces := []string{"cd", shellQuote(wslPath), "&&", "exec", name}
			for _, a := range args {
				pieces = append(pieces, shellQuote(a))
			}
			cmd := exec.Command("wsl", "-d", distro, "--", "bash", "-lc", strings.Join(pieces, " "))
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}
			return cmd
		}
	}
	cmd := exec.Command(name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}
	return cmd
}

func Run(ctx context.Context, name string, args []string, cwd string) error {
	cmd := execCommand(name, args, cwd)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()

	if stdoutBuf.Len() > 0 {
		output := stdoutBuf.String()
		if goruntime.GOOS == "windows" {
			output = decodeWinOutput(stdoutBuf.Bytes())
		}
		wailsruntime.EventsEmit(ctx, "log", filterWSLOutput(output))
	}

	if stderrBuf.Len() > 0 {
		output := stderrBuf.String()
		if goruntime.GOOS == "windows" {
			output = decodeWinOutput(stderrBuf.Bytes())
		}
		wailsruntime.EventsEmit(ctx, "log", filterWSLOutput(output))
	}

	return err
}

func filterWSLOutput(s string) string {
	lines := strings.Split(s, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "\ufeff")
		if strings.HasPrefix(strings.ToLower(trimmed), "wsl:") {
			continue
		}
		if hasLowControl(trimmed) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func hasLowControl(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\r' {
			return true
		}
	}
	return false
}

func decodeWinOutput(data []byte) string {
	utf8 := string(data)
	if strings.ContainsRune(utf8, '\uFFFD') {
		reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
		decoded, err := io.ReadAll(reader)
		if err == nil {
			return string(decoded)
		}
	}
	return utf8
}

func RunStreaming(ctx context.Context, name string, args []string, cwd string) error {
	cmd := execCommand(name, args, cwd)

	cmd.Stdout = &writer{ctx: ctx}
	cmd.Stderr = &writer{ctx: ctx}

	return cmd.Run()
}

type writer struct {
	ctx context.Context
}

func (w *writer) Write(p []byte) (n int, err error) {
	text := filterWSLOutput(string(p))
	if text != "" {
		wailsruntime.EventsEmit(w.ctx, "log", text)
	}
	return len(p), nil
}
