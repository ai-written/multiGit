package npm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"depdash/internal/command"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type PackageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func ParsePackageSpec(spec string) (PackageInfo, error) {
	idx := strings.LastIndex(spec, "@")
	if idx <= 0 {
		return PackageInfo{}, fmt.Errorf("invalid package format: %s", spec)
	}
	ver := spec[idx+1:]
	if ver == "" {
		return PackageInfo{}, fmt.Errorf("empty version in package spec: %s", spec)
	}
	return PackageInfo{Name: spec[:idx], Version: ver}, nil
}

func VerifyPackage(ctx context.Context, spec string, registry string) (bool, error) {
	runtime.EventsEmit(ctx, "log", fmt.Sprintf("验证中 %s...", spec))
	err := command.Run(ctx, "npm", []string{"view", spec, "version", "--registry=" + registry}, "")
	if err != nil {
		runtime.EventsEmit(ctx, "log", fmt.Sprintf("验证失败: %s", spec))
	}
	return err == nil, err
}

type depUpdate struct {
	section string
	name    string
	version string
}

type byteRange struct {
	start int64
	end   int64
}

type replacement struct {
	r     byteRange
	value []byte
}

func ApplyUpdates(cwd string, packages []string, bumpVersion bool, depType string) ([]string, error) {
	pkgPath := filepath.Join(cwd, "package.json")
	raw, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read package.json: %w", err)
	}

	var updates []depUpdate
	for _, spec := range packages {
		info, err := ParsePackageSpec(spec)
		if err != nil {
			return nil, err
		}
		updates = append(updates, depUpdate{name: info.Name, version: info.Version, section: depType})
	}

	var bumpedVersion string
	if bumpVersion {
		if ver, ok := topLevelString(raw, "version"); ok {
			re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(.*)$`)
			match := re.FindStringSubmatch(ver)
			if match != nil {
				patch := 0
				if _, err := fmt.Sscanf(match[3], "%d", &patch); err == nil {
					bumpedVersion = fmt.Sprintf(`%s.%s.%d%s`, match[1], match[2], patch+1, match[4])
				}
			}
		}
	}

	result, updated := patchJSON(raw, updates, bumpedVersion)
	if err := os.WriteFile(pkgPath, result, 0644); err != nil {
		return nil, fmt.Errorf("failed to write package.json: %w", err)
	}
	return updated, nil
}

func patchJSON(raw []byte, updates []depUpdate, bumpedVersion string) ([]byte, []string) {
	var replacements []replacement
	var updated []string
	handled := make(map[string]bool)

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var path []string
	var currentKey string
	expectValue := false

	for {
		before := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return raw, nil
		}
		after := dec.InputOffset()

		switch v := tok.(type) {
		case json.Delim:
			if v == '{' {
				if expectValue && currentKey != "" {
					path = append(path, currentKey)
				}
				expectValue = false
			} else if v == '}' {
				if len(path) > 0 {
					path = path[:len(path)-1]
				}
				expectValue = false
			}
		case string:
			if !expectValue {
				currentKey = v
				expectValue = true
			} else {
				path = append(path, currentKey)

				for before < after && raw[before] != '"' {
					before++
				}

				strVal := string(raw[before:after])
				inner := strVal
				if len(inner) >= 2 && inner[0] == '"' && inner[len(inner)-1] == '"' {
					inner = inner[1 : len(inner)-1]
				}

				if isPath(path, "version") && bumpedVersion != "" {
					newVal := `"` + escapeJSON(bumpedVersion) + `"`
					if strVal != newVal {
						replacements = append(replacements, replacement{r: byteRange{before, after}, value: []byte(newVal)})
					}
				}

				for _, u := range updates {
					if handled[u.name] {
						continue
					}
					if isPath(path, "dependencies", u.name) || isPath(path, "devDependencies", u.name) {
						if inner != u.version {
							newVal := `"` + escapeJSON(u.version) + `"`
							replacements = append(replacements, replacement{r: byteRange{before, after}, value: []byte(newVal)})
							updated = append(updated, u.name+"@"+u.version)
						}
						handled[u.name] = true
						break
					}
				}
				path = path[:len(path)-1]
				expectValue = false
			}
		default:
			if expectValue && currentKey != "" {
				path = append(path, currentKey)
				path = path[:len(path)-1]
			}
			expectValue = false
		}
	}

	result := applyReplacements(raw, replacements)

	unmatched := make(map[string][]depUpdate)
	for _, u := range updates {
		if !handled[u.name] {
			unmatched[u.section] = append(unmatched[u.section], u)
		}
	}
	for section, pkgs := range unmatched {
		result = insertNewPkgs(result, section, pkgs)
		for _, p := range pkgs {
			updated = append(updated, p.name+"@"+p.version)
		}
	}
	return result, updated
}

func isPath(path []string, parts ...string) bool {
	if len(path) != len(parts) {
		return false
	}
	for i, p := range parts {
		if path[i] != p {
			return false
		}
	}
	return true
}

func applyReplacements(raw []byte, replacements []replacement) []byte {
	for i := len(replacements) - 1; i >= 0; i-- {
		r := replacements[i]
		raw = append(raw[:r.r.start], append(r.value, raw[r.r.end:]...)...)
	}
	return raw
}

func insertNewPkgs(raw []byte, section string, pkgs []depUpdate) []byte {
	closePos, entryIndent := findClose(raw, section)
	if closePos < 0 {
		return raw
	}
	if entryIndent == "" {
		entryIndent = "        "
	}

	existing := sectionKeysRaw(raw, section)
	var toInsert []depUpdate
	for _, p := range pkgs {
		if !existing[p.name] {
			toInsert = append(toInsert, p)
		}
	}
	if len(toInsert) == 0 {
		return raw
	}

	needComma := false
	j := closePos - 1
	for j >= 0 && (raw[j] == ' ' || raw[j] == '\t' || raw[j] == '\n' || raw[j] == '\r') {
		j--
	}
	if j >= 0 && raw[j] != '{' {
		needComma = true
	}

	var insert bytes.Buffer
	for i, p := range toInsert {
		if i > 0 {
			insert.WriteByte(',')
		}
		fmt.Fprintf(&insert, "\n%s\"%s\": \"%s\"", entryIndent, escapeJSON(p.name), escapeJSON(p.version))
	}
	insert.WriteByte('\n')
	insert.WriteString(braceLineIndent(raw, closePos))

	result := make([]byte, 0, len(raw)+insert.Len()+1)
	result = append(result, raw[:j+1]...)
	if needComma {
		result = append(result, ',')
	}
	result = append(result, insert.Bytes()...)
	result = append(result, raw[closePos:]...)
	return result
}

func findClose(raw []byte, section string) (closePos int, entryIndent string) {
	key := []byte(`"` + section + `"`)
	pos := bytes.Index(raw, key)
	if pos < 0 {
		return -1, ""
	}
	i := pos + len(key)
	for i < len(raw) && raw[i] != '{' {
		i++
	}
	if i >= len(raw) {
		return -1, ""
	}
	bracePos := i
	depth := 1
	i++
	for i < len(raw) && depth > 0 {
		switch raw[i] {
		case '"':
			i++
			for i < len(raw) {
				if raw[i] == '\\' {
					i++
				} else if raw[i] == '"' {
					break
				}
				i++
			}
		case '{':
			depth++
		case '}':
			depth--
		}
		i++
	}
	if depth != 0 {
		return -1, ""
	}
	closePos = i - 1

	entryIndent = detectEntryIndent(raw, bracePos)
	return
}

func detectEntryIndent(raw []byte, bracePos int) string {
	i := bracePos + 1
	for i < len(raw) && raw[i] != '\n' && raw[i] != '}' {
		i++
	}
	if i < len(raw) && raw[i] == '}' {
		return ""
	}
	if i >= len(raw) {
		return ""
	}
	i++
	start := i
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
		i++
	}
	if start == i {
		return ""
	}
	return string(raw[start:i])
}

func braceLineIndent(raw []byte, closePos int) string {
	start := closePos
	for start > 0 && raw[start-1] != '\n' {
		start--
	}
	return string(raw[start:closePos])
}

func sectionKeysRaw(raw []byte, section string) map[string]bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	result := make(map[string]bool)
	inKey := true
	depth := 0
	foundKey := false
	sectionDepth := -1

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch v := tok.(type) {
		case json.Delim:
			if v == '{' {
				if foundKey && depth == 1 {
					sectionDepth = depth + 1
				}
				depth++
				inKey = true
			} else if v == '}' {
				depth--
				if depth == 1 {
					foundKey = false
					sectionDepth = -1
				}
			}
		case string:
			if inKey && depth == 1 && v == section {
				foundKey = true
			}
			if foundKey && depth == 2 && inKey && sectionDepth == depth {
				result[v] = true
			}
			inKey = !inKey
		default:
			inKey = true
		}
	}
	return result
}

func topLevelString(raw []byte, key string) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	inKey := true
	depth := 0
	foundKey := false

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch v := tok.(type) {
		case json.Delim:
			if v == '{' {
				depth++
				inKey = true
			} else if v == '}' {
				depth--
				inKey = false
			}
		case string:
			if depth == 1 && inKey && v == key {
				foundKey = true
				inKey = false
			} else if depth == 1 && !inKey && foundKey {
				return v, true
			} else {
				inKey = !inKey
			}
		default:
			if depth == 1 && !inKey && foundKey {
				inKey = true
			} else {
				inKey = true
			}
		}
	}
	return "", false
}

func escapeJSON(s string) string {
	escaped, _ := json.Marshal(s)
	return string(escaped[1 : len(escaped)-1])
}

func Install(ctx context.Context, cwd string, registry string) error {
	runtime.EventsEmit(ctx, "log", "[npm] 执行 npm install...")
	return command.Run(ctx, "npm", []string{"install", "--registry=" + registry}, cwd)
}
