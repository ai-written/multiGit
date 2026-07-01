package command

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	goruntime "runtime"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func Run(ctx context.Context, name string, args []string, cwd string) error {
	cmd := exec.Command(name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

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
	cmd := exec.Command(name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

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
