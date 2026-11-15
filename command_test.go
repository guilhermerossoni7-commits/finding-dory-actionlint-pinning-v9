package actionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandMain(t *testing.T) {
	var output bytes.Buffer

	// Create command instance populating stdin/stdout/stderr
	cmd := Command{
		Stdin:  os.Stdin,
		Stdout: &output,
		Stderr: &output,
	}

	// Run the command end-to-end. Note that given args should contain program name
	workflow := filepath.Join("testdata", "examples", "main.yaml")
	status := cmd.Main([]string{"actionlint", "-shellcheck=", "-pyflakes=", "-ignore", `label .+ is unknown\.`, workflow})

	if status != 1 {
		t.Fatal("exit status should be 1 but got", status)
	}

	out := output.String()

	for _, s := range []string{
		"main.yaml:3:5:",
		"unexpected key \"branch\" for \"push\" section",
		"^~~~~~~~~~~~~~~",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("output should contain %q: %q", s, out)
		}
	}

	if strings.Contains(out, "[runner-label]") {
		t.Errorf("runner-label rule should be ignored by -ignore but it is included in output: %q", out)
	}
}

func TestCommandActionPinningLevel(t *testing.T) {
	for _, tc := range []struct {
		level               string
		status, diagnostics int
	}{
		{"major-minor", 0, 0},
		{"semver", 1, 1},
		{"commit-sha", 1, 3},
		{"unknown", 2, 0},
		{"", 2, 0},
	} {
		t.Run(tc.level, func(t *testing.T) {
			var output bytes.Buffer
			cmd := Command{Stdin: strings.NewReader(actionPinningTestWorkflow), Stdout: &output, Stderr: &output}
			status := cmd.Main([]string{"actionlint", "-shellcheck=", "-pyflakes=", "-no-color", "-action-pinning-level=" + tc.level, "-"})
			if status != tc.status || strings.Count(output.String(), "[action-pinning]") != tc.diagnostics {
				t.Fatalf("got status=%d output=%s; want status=%d diagnostics=%d", status, &output, tc.status, tc.diagnostics)
			}
			if tc.status == 2 && !strings.Contains(output.String(), "invalid action pinning level") {
				t.Fatal("missing invalid level diagnostic:", &output)
			}
		})
	}
}
