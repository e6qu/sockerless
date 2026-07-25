package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// iniKV is one `key = value` line of the profile section the login writes.
// Order is preserved in the file.
type iniKV struct {
	Key   string
	Value string
}

// upsertINISection rewrites exactly one `[section]` of an INI file, leaving
// every other byte of the file untouched. A missing file is created; a missing
// section is appended. The section's previous body is fully replaced — the
// section is machine-owned by `sockerless login`.
func upsertINISection(path, section string, values []iniKV) error {
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var body strings.Builder
	body.WriteString("[" + section + "]\n")
	for _, kv := range values {
		fmt.Fprintf(&body, "%s = %s\n", kv.Key, kv.Value)
	}

	updated, found := replaceINISection(string(content), section, body.String())
	if !found {
		updated = string(content)
		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		if updated != "" {
			updated += "\n"
		}
		updated += body.String()
	}
	return writePreservingMode(path, []byte(updated))
}

// removeINISection deletes one `[section]` from an INI file, leaving the rest
// untouched. A missing file or section is not an error; found reports whether
// the section existed.
func removeINISection(path, section string) (found bool, err error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	updated, found := replaceINISection(string(content), section, "")
	if !found {
		return false, nil
	}
	return true, writePreservingMode(path, []byte(updated))
}

// replaceINISection swaps the body of `[section]` (header line through the
// line before the next section header or end of file) for replacement, which
// must carry its own header when non-empty. It reports whether the section
// was present.
func replaceINISection(content, section, replacement string) (string, bool) {
	lines := strings.SplitAfter(content, "\n")
	header := "[" + section + "]"
	start := -1
	end := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 {
			if trimmed == header {
				start = i
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			end = i
			break
		}
	}
	if start == -1 {
		return content, false
	}
	// Trailing blank lines between our section and the next belong to the
	// separator, not to our section body — keep exactly one when a section
	// follows and the replacement is non-empty.
	before := strings.Join(lines[:start], "")
	after := strings.Join(lines[end:], "")
	var out strings.Builder
	out.WriteString(before)
	out.WriteString(replacement)
	if replacement != "" && after != "" && !strings.HasPrefix(after, "\n") {
		out.WriteString("\n")
	}
	out.WriteString(after)
	return out.String(), true
}

func writePreservingMode(path string, content []byte) error {
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// awsConfigFilePath resolves the aws CLI config file the way the aws CLI
// does: AWS_CONFIG_FILE, else ~/.aws/config.
func awsConfigFilePath() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}
