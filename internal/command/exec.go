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
			cmd := exec.Command("wsl", "-d", distro, "--", "sh", "-c", strings.Join(pieces, " "))
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

	if goruntime.GOOS == "windows" {
		if stdoutBuf.Len() > 0 {
			wailsruntime.EventsEmit(ctx, "log", decodeWinOutput(stdoutBuf.Bytes()))
		}
	} else {
		if stdoutBuf.Len() > 0 {
			wailsruntime.EventsEmit(ctx, "log", stdoutBuf.String())
		}
	}

	if stderrBuf.Len() > 0 {
		output := stderrBuf.String()
		if goruntime.GOOS == "windows" {
			output = decodeWinOutput(stderrBuf.Bytes())
		}
		wailsruntime.EventsEmit(ctx, "log", output)
	}

	return err
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
	text := string(p)
	wailsruntime.EventsEmit(w.ctx, "log", text)
	return len(p), nil
}
