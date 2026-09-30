package util

import (
	"bufio"
	"os"
	"sort"
	"strconv"
	"strings"
)

// ParseConfFile parses a Luanti "Settings" style file:
// one `key = value` per line
func ParseConfFile(data []byte) map[string]string {
	result := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return result
}

// ParseIntField reads an integer field from a parsed conf map,
// defaulting to 0 if the key is missing or not a valid integer.
func ParseIntField(conf map[string]string, key string) int {
	n, err := strconv.Atoi(conf[key])
	if err != nil {
		return 0
	}
	return n
}

// SplitList splits a comma-separated mod.conf list field (depends,
// optional_depends), trimming whitespace and dropping empty entries.
func SplitList(value string) []string {
	if value == "" {
		return nil
	}

	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}

	return out
}

func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// SetConfFields sets keys in the conf file at path, replacing existing lines
// in place and appending missing ones. File is created if absent.
func SetConfFields(path string, fields map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	remaining := make(map[string]string, len(fields))
	for k, v := range fields {
		remaining[k] = v
	}

	var lines []string
	if len(data) > 0 {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, _, found := strings.Cut(trimmed, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		if value, ok := remaining[key]; ok {
			lines[i] = key + " = " + value
			delete(remaining, key)
		}
	}

	keys := make([]string, 0, len(remaining))
	for k := range remaining {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lines = append(lines, k+" = "+remaining[k])
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
