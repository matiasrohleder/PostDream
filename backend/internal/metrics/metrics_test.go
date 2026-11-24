package metrics

import (
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	Register("test_total", "A test counter.", "inbox")
	Inc("test_total", `qa"x`)
	Add("test_total", 2, `qa"x`)
	var b strings.Builder
	Write(&b, []Gauge{{Name: "test_gauge", Help: "g", Labels: map[string]string{"inbox": "a"}, Value: 3}})
	out := b.String()
	for _, want := range []string{
		"# TYPE test_total counter", `test_total{inbox="qa\"x"} 3`,
		"# TYPE test_gauge gauge", `test_gauge{inbox="a"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
