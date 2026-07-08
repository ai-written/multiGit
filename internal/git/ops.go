package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"io"
	"multigit/internal/command"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func parseWSLPath(p string) (distro, wslPath string, ok bool) {
	p = filepath.FromSlash(p)
	parts := strings.SplitN(p, `\`, 5)
	if len(parts) >= 4 && strings.HasPrefix(strings.ToLower(p), `\\wsl`) {
		return parts[3], "/" + strings.ReplaceAll(parts[4], `\`, "/"), true
	}
	return "", "", false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func gitCmd(cwd string, args ...string) *exec.Cmd {
	if distro, wslPath, ok := parseWSLPath(cwd); ok {
		pieces := []string{"exec", "git", "-C", shellQuote(wslPath)}
		for _, a := range args {
			pieces = append(pieces, shellQuote(a))
		}
		cmd := exec.Command("wsl", "-d", distro, "--", "sh", "-c", strings.Join(pieces, " "))
		cmd.Stderr = io.Discard
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		return cmd
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func parseRefs(s string) []string {
	if s == "" {
		return nil
	}
	var tags []string
	for _, part := range strings.Split(s, ", ") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "tag: ") {
			tags = append(tags, strings.TrimPrefix(part, "tag: "))
		}
	}
	return tags
}

type CommitInfo struct {
	Hash      string   `json:"hash"`
	Author    string   `json:"author"`
	Date      string   `json:"date"`
	Message   string   `json:"message"`
	Timestamp int64    `json:"-"`
	Tags      []string `json:"tags,omitempty"`
	DateISO   string   `json:"dateISO,omitempty"`
	Parents   []string `json:"parents,omitempty"`
}

type CommitDetail struct {
	Hash          string `json:"hash"`
	Author        string `json:"author"`
	AuthorEmail   string `json:"authorEmail"`
	AuthorDate    string `json:"authorDate"`
	Committer     string `json:"committer"`
	CommitterEmail string `json:"committerEmail"`
	CommitterDate string   `json:"committerDate"`
	Message       string   `json:"message"`
	Tags          []string `json:"tags,omitempty"`
}

type FileChange struct {
	Status   string `json:"status"`
	FilePath string `json:"filePath"`
}

type DiffLine struct {
	Type    string `json:"type"`
	OldLine string `json:"oldLine"`
	NewLine string `json:"newLine"`
	OldNum  int    `json:"oldNum"`
	NewNum  int    `json:"newNum"`
}

type DiffHunk struct {
	Lines []DiffLine `json:"lines"`
}

type BranchDiffStat struct {
	CommitsAhead  int `json:"commitsAhead"`
	CommitsBehind int `json:"commitsBehind"`
	FilesAdded    int `json:"filesAdded"`
	FilesModified int `json:"filesModified"`
	FilesDeleted  int `json:"filesDeleted"`
	LinesAdded    int `json:"linesAdded"`
	LinesDeleted  int `json:"linesDeleted"`
}

func GetBranchDiffStat(cwd, branchA, branchB string) (*BranchDiffStat, error) {
	// Run git cmd with fallback to origin/<branch>
	run := func(args ...string) ([]byte, error) {
		out, err := gitCmd(cwd, args...).Output()
		if err == nil {
			return out, nil
		}
		// Try with local args first failed - fallback will be handled in specific calls
		return nil, err
	}

	withOrigin := func(a string) string {
		switch {
		case a == branchA:
			return "origin/" + a
		case a == branchB:
			return "origin/" + a
		case a == "^"+branchA:
			return "^origin/" + branchA
		case a == "^"+branchB:
			return "^origin/" + branchB
		case a == branchA+".."+branchB:
			return "origin/" + branchA + ".." + "origin/" + branchB
		default:
			return a
		}
	}

	runWithFallback := func(cmd string, args ...string) ([]byte, error) {
		fullArgs := append([]string{cmd}, args...)
		out, err := run(fullArgs...)
		if err == nil {
			return out, nil
		}
		fallbackArgs := make([]string, len(args))
		for i, a := range args {
			fallbackArgs[i] = withOrigin(a)
		}
		fullFallback := append([]string{cmd}, fallbackArgs...)
		return run(fullFallback...)
	}

	parseInt := func(b []byte) int {
		var n int
		if len(b) == 0 {
			return 0
		}
		fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &n)
		return n
	}

	aheadOut, aheadErr := runWithFallback("rev-list", "--count", branchA, "^"+branchB)
	behindOut, behindErr := runWithFallback("rev-list", "--count", branchB, "^"+branchA)
	if aheadErr != nil || behindErr != nil {
		return nil, fmt.Errorf("one or both branches not found in this repository")
	}
	ahead := parseInt(aheadOut)
	behind := parseInt(behindOut)

	diffRef := branchA + ".." + branchB
	numstatOut, numstatErr := runWithFallback("diff", "--numstat", diffRef)
	if numstatErr != nil {
		numstatOut, _ = run("diff", "--numstat", withOrigin(diffRef))
	}

	var stat BranchDiffStat
	stat.CommitsAhead = ahead
	stat.CommitsBehind = behind

	for _, line := range strings.Split(strings.TrimSpace(string(numstatOut)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		added := parseInt([]byte(parts[0]))
		deleted := parseInt([]byte(parts[1]))
		stat.LinesAdded += added
		stat.LinesDeleted += deleted
	}

	statusOut, statusErr := runWithFallback("diff", "--name-status", diffRef)
	if statusErr != nil {
		statusOut, _ = run("diff", "--name-status", withOrigin(diffRef))
	}
	for _, line := range strings.Split(strings.TrimSpace(string(statusOut)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}
		switch parts[0][0] {
		case 'A':
			stat.FilesAdded++
		case 'M', 'R':
			stat.FilesModified++
		case 'D':
			stat.FilesDeleted++
		}
	}

	return &stat, nil
}

func CurrentBranch(cwd string) (string, error) {
	cmd := gitCmd(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func IsRepo(cwd string) bool {
	path := filepath.Join(cwd, ".git")
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return true
	}
	// 子模块：.git 是文件，内容为 "gitdir: ../.git/modules/xxx"
	data, _ := os.ReadFile(path)
	return bytes.HasPrefix(data, []byte("gitdir: "))
}

func statusHasChanges(cwd string) (bool, error) {
	cmd := gitCmd(cwd, "status", "--porcelain")
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
	cmd := gitCmd(cwd, "stash", "list")
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
	cmd := gitCmd(cwd, "branch")
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
	cmd := gitCmd(cwd, "branch", "-r")
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

func ListBranches(cwd string) ([]string, error) {
	seen := map[string]bool{}
	var branches []string

	cmd := gitCmd(cwd, "branch", "-a", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		trimmed := strings.TrimPrefix(line, "remotes/")
		trimmed = strings.TrimPrefix(trimmed, "origin/")
		if !seen[trimmed] && trimmed != "" && trimmed != "origin" && !strings.Contains(trimmed, " -> ") {
			seen[trimmed] = true
			branches = append(branches, trimmed)
		}
	}

	return branches, nil
}

func Fetch(ctx context.Context, cwd string) error {
	runtime.EventsEmit(ctx, "log", "[git] 正在拉取远程...")
	err := command.Run(ctx, "git", []string{"fetch", "origin"}, cwd)
	if err == nil {
		runtime.EventsEmit(ctx, "log", "[git] 远程拉取完成")
	}
	return err
}

func Log(cwd, ref string, n, skip int) ([]CommitInfo, error) {
	cmd := gitCmd(cwd, "log", ref, fmt.Sprintf("--skip=%d", skip), fmt.Sprintf("-n%d", n), "--date=format:%Y-%m-%d %H:%M:%S", "--format=%H%x00%an%x00%ar%x00%s%x00%D%x00%ad%x00%P")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 7)
		if len(parts) >= 4 {
			c := CommitInfo{Hash: parts[0], Author: parts[1], Date: parts[2], Message: parts[3]}
			if len(parts) >= 5 {
				c.Tags = parseRefs(parts[4])
			}
			if len(parts) >= 6 {
				c.DateISO = parts[5]
			}
			if len(parts) >= 7 {
				c.Parents = strings.Fields(parts[6])
			}
			commits = append(commits, c)
		}
	}
	return commits, nil
}

func SearchCommitsPage(cwd, ref, term string, pageSize, skip int) ([]CommitInfo, bool, error) {
	lower := strings.ToLower(term)
	seen := map[string]bool{}

	results := make([]CommitInfo, 0)

	addIfNew := func(c CommitInfo) {
		if !seen[c.Hash] {
			seen[c.Hash] = true
			results = append(results, c)
		}
	}

	format := "%H%x00%an%x00%ar%x00%s%x00%ct%x00%D%x00%ad"
	dateArg := "--date=format:%Y-%m-%d %H:%M:%S"

	runCmd := func(args ...string) ([]CommitInfo, error) {
		cmd := gitCmd(cwd, args...)
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var commits []CommitInfo
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\x00", 7)
			if len(parts) >= 5 {
				ts, _ := strconv.ParseInt(parts[4], 10, 64)
				c := CommitInfo{Hash: parts[0], Author: parts[1], Date: parts[2], Message: parts[3], Timestamp: ts}
				if len(parts) >= 6 {
					c.Tags = parseRefs(parts[5])
				}
				if len(parts) >= 7 {
					c.DateISO = parts[6]
				}
				commits = append(commits, c)
			}
		}
		return commits, nil
	}

	// 1. Search by message
	msgCommits, _ := runCmd("log", ref, "--regexp-ignore-case", "--grep", term, fmt.Sprintf("--skip=%d", skip), fmt.Sprintf("-n%d", pageSize), dateArg, "--format="+format)
	for _, c := range msgCommits {
		addIfNew(c)
	}

	// 2. Search by author
	authorCommits, _ := runCmd("log", ref, "--regexp-ignore-case", "--author", term, fmt.Sprintf("--skip=%d", skip), fmt.Sprintf("-n%d", pageSize), dateArg, "--format="+format)
	for _, c := range authorCommits {
		addIfNew(c)
	}

	// 3. Search by hash prefix
	hashCmd := gitCmd(cwd, "rev-list", ref, "--max-count=5000")
	hashOut, hashErr := hashCmd.Output()
	if hashErr == nil {
		var matchedHashes []string
		for _, h := range strings.Split(strings.TrimSpace(string(hashOut)), "\n") {
			if h != "" && strings.HasPrefix(strings.ToLower(h), lower) {
				matchedHashes = append(matchedHashes, h)
			}
		}
		for i := skip; i < len(matchedHashes) && len(results)-len(msgCommits)-len(authorCommits) < pageSize; i++ {
			c, _ := runCmd("log", ref, "-n1", dateArg, "--format="+format, matchedHashes[i])
			if len(c) > 0 {
				addIfNew(c[0])
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})

	hasMore := len(results) >= pageSize
	if len(results) > pageSize {
		results = results[:pageSize]
	}
	return results, hasMore, nil
}

func IsAncestor(cwd, hash, branch string) (bool, error) {
	cmd := gitCmd(cwd, "merge-base", "--is-ancestor", hash, branch)
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

func ForcePush(ctx context.Context, cwd string, branch string) error {
	runtime.EventsEmit(ctx, "log", "[git] 强制推送到 origin/"+branch+"...")
	return command.Run(ctx, "git", []string{"push", "--force", "origin", branch}, cwd)
}

func ForceCheckoutTo(ctx context.Context, cwd string, targetBranch string, sourceRef string) error {
	runtime.EventsEmit(ctx, "log", "[git] 强制检出分支: "+targetBranch+" -> "+sourceRef)
	return command.Run(ctx, "git", []string{"checkout", "-B", targetBranch, sourceRef}, cwd)
}

func branchExistsLocal(cwd string, branch string) (bool, error) {
	localBranches, err := listLocalBranches(cwd)
	if err != nil {
		return false, err
	}
	for _, b := range localBranches {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}

func BackupBranch(ctx context.Context, cwd string, targetBranch string) (string, error) {
	backupName := targetBranch + "-backup"

	existsLocal, err := branchExistsLocal(cwd, targetBranch)
	if err != nil {
		return "", err
	}
	if existsLocal {
		runtime.EventsEmit(ctx, "log", fmt.Sprintf("[git] 备份目标分支 %s -> %s", targetBranch, backupName))
		err := command.Run(ctx, "git", []string{"branch", "-f", backupName, targetBranch}, cwd)
		if err != nil {
			return "", err
		}
		runtime.EventsEmit(ctx, "log", "[git] 备份完成")
		return backupName, nil
	}

	remoteBranches, err := listRemoteBranches(cwd)
	if err != nil {
		return "", err
	}
	remoteFull := "origin/" + targetBranch
	for _, b := range remoteBranches {
		if b == remoteFull {
			runtime.EventsEmit(ctx, "log", fmt.Sprintf("[git] 备份远程目标分支 %s -> %s", targetBranch, backupName))
			err := command.Run(ctx, "git", []string{"branch", "-f", backupName, remoteFull}, cwd)
			if err != nil {
				return "", err
			}
			runtime.EventsEmit(ctx, "log", "[git] 备份完成")
			return backupName, nil
		}
	}

	return "", nil
}

func RestoreBackupBranches(ctx context.Context, cwd string) ([]string, error) {
	localBranches, err := listLocalBranches(cwd)
	if err != nil {
		return nil, err
	}

	var backupBranches []string
	for _, b := range localBranches {
		if strings.HasSuffix(b, "-backup") {
			backupBranches = append(backupBranches, b)
		}
	}
	if len(backupBranches) == 0 {
		return nil, nil
	}

	var restored []string
	for _, backup := range backupBranches {
		original := strings.TrimSuffix(backup, "-backup")
		runtime.EventsEmit(ctx, "log", fmt.Sprintf("[git] 还原: %s -> %s", backup, original))
		if err := command.Run(ctx, "git", []string{"checkout", "-B", original, backup}, cwd); err != nil {
			return restored, fmt.Errorf("还原 %s -> %s 失败: %w", backup, original, err)
		}
		runtime.EventsEmit(ctx, "log", "[git] 强制推送 "+original)
		if err := command.Run(ctx, "git", []string{"push", "--force", "origin", original}, cwd); err != nil {
			return restored, fmt.Errorf("推送 %s 失败: %w", original, err)
		}
		restored = append(restored, original+" <- "+backup)
	}

	return restored, nil
}

func GetCommitFiles(cwd, hash string) ([]FileChange, error) {
	cmd := gitCmd(cwd, "diff-tree", "--no-commit-id", "-r", "--name-status", "-z", hash)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	parts := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	var files []FileChange
	for i := 0; i+1 < len(parts); i += 2 {
		files = append(files, FileChange{Status: parts[i], FilePath: parts[i+1]})
	}
	return files, nil
}

func GetCommitFileContent(cwd, hash, filePath string) (string, error) {
	cmd := gitCmd(cwd, "show", hash+":"+filePath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func GetCommitDetail(cwd, hash string) (*CommitDetail, error) {
	cmd := gitCmd(cwd, "show", "--no-patch", "--date=format:%Y-%m-%d %H:%M:%S", "--format=%H%x00%an%x00%ae%x00%ad%x00%cn%x00%ce%x00%cd%x00%s%x00%D", hash)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\x00", 9)
	if len(parts) < 8 {
		return nil, fmt.Errorf("unexpected output: %s", string(out))
	}
	d := &CommitDetail{
		Hash:           parts[0],
		Author:         parts[1],
		AuthorEmail:    parts[2],
		AuthorDate:     parts[3],
		Committer:      parts[4],
		CommitterEmail: parts[5],
		CommitterDate:  parts[6],
		Message:        parts[7],
	}
	if len(parts) >= 9 {
		d.Tags = parseRefs(parts[8])
	}
	return d, nil
}

func GetCommitFileDiff(cwd, hash, filePath string) (string, error) {
	cmd := gitCmd(cwd, "show", "--no-color", hash, "--", filePath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func isFileTooLarge(cwd, hash, filePath string) bool {
	if strings.Contains(filePath, ".min.") || strings.Contains(filePath, "-min.") {
		return true
	}
	cmd := gitCmd(cwd, "show", hash+":"+filePath)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	if len(out) > 500*1024 {
		return true
	}
	lines := strings.Count(string(out), "\n")
	if lines > 5000 {
		return true
	}
	maxLine := 0
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) > maxLine {
			maxLine = len(line)
		}
	}
	if maxLine > 1000 {
		return true
	}
	return false
}

func GetCommitFileDiffSideBySide(cwd, hash, filePath string) ([]DiffHunk, bool, error) {
	if isFileTooLarge(cwd, hash, filePath) {
		return nil, true, nil
	}

	parentCmd := gitCmd(cwd, "rev-parse", hash+"^")
	parentOut, parentErr := parentCmd.Output()

	parent := strings.TrimSpace(string(parentOut))
	if parentErr != nil {
		parent = "4b825dc642cb6eb9a060e54bf899d153036f71af" // empty tree
	}

	diffArgs := []string{"diff", parent, hash, "--", filePath, "-U3", "--no-color"}
	cmd := gitCmd(cwd, diffArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, false, err
	}

	lines := strings.Split(string(out), "\n")
	var hunks []DiffHunk
	var currentHunk *DiffHunk
	oldNum, newNum := 0, 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}
			currentHunk = &DiffHunk{}
			parts := strings.Split(line, " ")
			if len(parts) >= 4 {
				oldStart := 0
				newStart := 0
				fmt.Sscanf(parts[1], "-%d", &oldStart)
				fmt.Sscanf(parts[2], "+%d", &newStart)
				oldNum = oldStart
				newNum = newStart
			}
			continue
		}
		if currentHunk == nil {
			continue
		}

		dl := DiffLine{OldNum: oldNum, NewNum: newNum}
		if len(line) == 0 {
			dl.Type = "context"
			dl.OldLine = ""
			dl.NewLine = ""
			oldNum++
			newNum++
		} else if line[0] == ' ' {
			dl.Type = "context"
			dl.OldLine = line[1:]
			dl.NewLine = line[1:]
			oldNum++
			newNum++
		} else if line[0] == '-' {
			dl.Type = "delete"
			dl.OldLine = line[1:]
			dl.NewLine = ""
			oldNum++
		} else if line[0] == '+' {
			dl.Type = "add"
			dl.OldLine = ""
			dl.NewLine = line[1:]
			newNum++
		} else {
			continue
		}
		currentHunk.Lines = append(currentHunk.Lines, dl)
	}

	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}
	return hunks, false, nil
}

func DeleteBackupBranches(ctx context.Context, cwd string) ([]string, error) {
	var deleted []string

	localBranches, err := listLocalBranches(cwd)
	if err != nil {
		return nil, err
	}
	for _, b := range localBranches {
		if strings.HasSuffix(b, "-backup") {
			runtime.EventsEmit(ctx, "log", "[git] 删除本地备份分支: "+b)
			err := command.Run(ctx, "git", []string{"branch", "-D", b}, cwd)
			if err != nil {
				runtime.EventsEmit(ctx, "log", "[git] 删除本地分支失败: "+b)
				continue
			}
			deleted = append(deleted, b)
		}
	}

	remoteBranches, err := listRemoteBranches(cwd)
	if err != nil {
		return deleted, err
	}
	for _, b := range remoteBranches {
		trimmed := strings.TrimPrefix(b, "origin/")
		if strings.HasSuffix(trimmed, "-backup") {
			runtime.EventsEmit(ctx, "log", "[git] 删除远程备份分支: "+trimmed)
			err := command.Run(ctx, "git", []string{"push", "origin", "--delete", trimmed}, cwd)
			if err != nil {
				runtime.EventsEmit(ctx, "log", "[git] 删除远程分支失败: "+trimmed)
				continue
			}
			deleted = append(deleted, "origin/"+trimmed)
		}
	}

	return deleted, nil
}
