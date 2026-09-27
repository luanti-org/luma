package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfFile(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected map[string]string
	}{
		{
			name:     "empty input",
			data:     []byte(""),
			expected: map[string]string{},
		},
		{
			name:     "simple key-value",
			data:     []byte("key = value"),
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "multiple lines",
			data:     []byte("a = 1\nb = 2\nc = 3"),
			expected: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name:     "comment lines skipped",
			data:     []byte("# comment\nkey = value"),
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "blank lines skipped",
			data:     []byte("\n\nkey = value\n"),
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "whitespace trimmed",
			data:     []byte("  key  =  value  "),
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "line without equals skipped",
			data:     []byte("invalid_line\nkey = value"),
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "hash comment skipped",
			data:     []byte("# this is a comment\nkey=value"),
			expected: map[string]string{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseConfFile(tt.data)
			if len(result) != len(tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("result[%q] = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

func TestParseIntField(t *testing.T) {
	tests := []struct {
		name     string
		conf     map[string]string
		key      string
		expected int
	}{
		{
			name:     "valid integer",
			conf:     map[string]string{"port": "8080"},
			key:      "port",
			expected: 8080,
		},
		{
			name:     "missing key returns 0",
			conf:     map[string]string{"port": "8080"},
			key:      "missing",
			expected: 0,
		},
		{
			name:     "invalid integer returns 0",
			conf:     map[string]string{"port": "abc"},
			key:      "port",
			expected: 0,
		},
		{
			name:     "zero value",
			conf:     map[string]string{"count": "0"},
			key:      "count",
			expected: 0,
		},
		{
			name:     "empty conf map",
			conf:     map[string]string{},
			key:      "key",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseIntField(tt.conf, tt.key)
			if result != tt.expected {
				t.Errorf("got %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected []string
	}{
		{
			name:     "empty string",
			value:    "",
			expected: nil,
		},
		{
			name:     "single item",
			value:    "mod_a",
			expected: []string{"mod_a"},
		},
		{
			name:     "comma-separated",
			value:    "mod_a,mod_b,mod_c",
			expected: []string{"mod_a", "mod_b", "mod_c"},
		},
		{
			name:     "whitespace trimmed",
			value:    "mod_a, mod_b , mod_c",
			expected: []string{"mod_a", "mod_b", "mod_c"},
		},
		{
			name:     "empty entries dropped",
			value:    "mod_a,,mod_b",
			expected: []string{"mod_a", "mod_b"},
		},
		{
			name:     "trailing comma",
			value:    "mod_a,mod_b,",
			expected: []string{"mod_a", "mod_b"},
		},
		{
			name:     "all empty",
			value:    ",,",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitList(tt.value)
			if len(result) != len(tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("result[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	tmpSubdir := filepath.Join(tmpDir, "testdir")
	if err := os.Mkdir(tmpSubdir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "existing file",
			path:     tmpFile,
			expected: true,
		},
		{
			name:     "directory does not count",
			path:     tmpSubdir,
			expected: false,
		},
		{
			name:     "nonexistent path",
			path:     filepath.Join(tmpDir, "nonexistent.txt"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FileExists(tt.path)
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}
