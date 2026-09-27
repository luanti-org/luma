package engine

import "testing"

func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		name, out, want string
		wantErr         bool
	}{
		{"luanti", "Luanti 5.17.0 (Windows)\nUsing LuaJIT 2.1\n", "5.17.0", false},
		{"minetest", "Minetest 5.4.1 (Linux)\n", "5.4.1", false},
		{"noise first", "warning: something\nLuanti 5.9.1 (Linux)\n", "5.9.1", false},
		{"dev", "Luanti 5.18.0-dev-abc123 (Linux)\n", "5.18.0-dev-abc123", false},
		{"crlf", "Luanti 5.17.0 (Windows)\r\nUsing LuaJIT\r\n", "5.17.0", false},
		{"garbage", "hello world\n", "", true},
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVersionOutput(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("version = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRunInPlace(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
		ok   bool
	}{
		{"in place", "Luanti 5.17.0 (Windows)\nBUILD_TYPE=Release\nRUN_IN_PLACE=1\n", true, true},
		{"not in place", "Luanti 5.17.0 (Linux)\nBUILD_TYPE=Release\nRUN_IN_PLACE=0\n", false, true},
		{"absent", "Luanti 5.17.0 (Linux)\n", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRunInPlace(tt.out)
			if got != tt.want || ok != tt.ok {
				t.Errorf("parseRunInPlace(...) = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

const miscS = `
local other = {
	["9.9.9"] = 1,
}

-- Protocol version table
core.protocol_versions = {
	["5.0.0"] = 37,
	["5.9.0"] = 44,
	["5.9.1"] = 45,
	["5.16.0"] = 52,
	["5.17.0"] = 53,
}

setmetatable(core.protocol_versions, {})
`

func TestParseProtocolTable(t *testing.T) {
	table, err := parseProtocolTable(miscS)
	if err != nil {
		t.Fatalf("parseProtocolTable: %v", err)
	}
	want := map[string]int{"5.0.0": 37, "5.9.0": 44, "5.9.1": 45, "5.16.0": 52, "5.17.0": 53}
	if len(table) != len(want) {
		t.Fatalf("table = %v, want %v", table, want)
	}
	for k, v := range want {
		if table[k] != v {
			t.Errorf("table[%q] = %d, want %d", k, table[k], v)
		}
	}
}

func TestParseProtocolTableMissing(t *testing.T) {
	if _, err := parseProtocolTable("-- nothing here\n"); err == nil {
		t.Error("expected error for text without a protocol table")
	}
}

func TestLookupProtocol(t *testing.T) {
	table := map[string]int{"5.9.0": 44, "5.9.1": 45, "5.17.0": 53}
	tests := []struct {
		version string
		want    int
		ok      bool
	}{
		{"5.17.0", 53, true},
		{"5.9.1", 45, true},
		{"5.17.1", 53, true},
		{"5.17", 53, true},
		{"5.9.2", 44, true},
		{"5.18.0-dev", 53, true}, // assumed to speak the newest known protocol
		{"5.17.0-rc1", 53, true},
		{"6.0.0", 0, false},
		{"5", 0, false},
	}
	for _, tt := range tests {
		got, ok := lookupProtocol(table, tt.version)
		if got != tt.want || ok != tt.ok {
			t.Errorf("lookupProtocol(%q) = %d, %v; want %d, %v", tt.version, got, ok, tt.want, tt.ok)
		}
	}
}
