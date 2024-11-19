package runner

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestDoesPatternMatch(t *testing.T) {
	table := []struct {
		pattern, data string
		shouldMatch   bool
	}{
		{pattern: "{}", data: "{}", shouldMatch: true},
		{pattern: `
a: b
b:
  - c: d
    e: f
`, data: `
a: b
b:
  - c: d
    e: f
    foo: bar
hoge: piyo
`, shouldMatch: true},
		{pattern: `{ "a": "b" }`, data: `{ "a": "c" }`, shouldMatch: false},
		{pattern: `
a:
  - b: c
    d: e
  `, data: `
a:
  - b: c
    d: f
`, shouldMatch: false},
	}

	for _, e := range table {
		var pattern, data interface{}
		if err := yaml.Unmarshal([]byte(e.pattern), &pattern); err != nil {
			t.Errorf("failed to parse pattern: %v: %s", err, e.pattern)
		}
		if err := yaml.Unmarshal([]byte(e.data), &data); err != nil {
			t.Errorf("failed to parse data: %v: %s", err, e.data)
		}
		if e.shouldMatch != doesPatternMatch(pattern, data) {
			t.Errorf("pattern match failed: %s: %s: %v", e.pattern, e.data, e.shouldMatch)
		}
	}
}
