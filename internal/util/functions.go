package util

import (
	"bufio"
	"os"
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
