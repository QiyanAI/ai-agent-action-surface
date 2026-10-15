package playground

import (
	"os"
	"strings"
	"testing"
)

func mustGoFiles(t *testing.T) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var imports []string
		inBlock := false
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "import (" {
				inBlock = true
				continue
			}
			if inBlock && line == ")" {
				inBlock = false
				continue
			}
			if inBlock && strings.HasPrefix(line, "\"") {
				imports = append(imports, strings.Trim(line, "\""))
			}
		}
		out[name] = imports
	}
	return out
}
