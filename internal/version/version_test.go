package version

import (
	"strings"
	"testing"
)

func TestUserAgentWithEngine(t *testing.T) {
	ua := UserAgent("5.17.0")
	if !strings.HasPrefix(ua, "Luma/"+String()+" (") {
		t.Errorf("UserAgent = %q, want Luma/<ver> (...) prefix", ua)
	}
	if !strings.HasSuffix(ua, ") Luanti/5.17.0") {
		t.Errorf("UserAgent = %q, want ) Luanti/5.17.0 suffix", ua)
	}
}

func TestUserAgentWithoutEngine(t *testing.T) {
	ua := UserAgent("")
	if strings.Contains(ua, "Luanti") || !strings.HasSuffix(ua, ")") {
		t.Errorf("UserAgent = %q, want no Luanti part", ua)
	}
}
