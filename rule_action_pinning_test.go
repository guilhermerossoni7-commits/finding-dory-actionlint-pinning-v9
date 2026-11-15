package actionlint

import (
	"fmt"
	"strings"
	"testing"
)

func TestRuleActionPinning(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		uses                    string
		majorMinor, semver, sha bool
	}{
		{"owner/repo@" + sha, true, true, true},
		{"owner/repo/path@" + sha, true, true, true},
		{"owner/repo@" + strings.ToUpper(sha), false, false, false},
		{"owner/repo@v4", false, false, false},
		{"owner/repo@4.2", true, false, false},
		{"owner/repo@v4.2", true, false, false},
		{"owner/repo@0.0", true, false, false},
		{"owner/repo@v4.2.1", true, true, false},
		{"owner/repo@4.2.1", true, true, false},
		{"owner/repo@0.0.0", true, true, false},
		{"owner/repo@v4.2.1-rc.1", true, true, false},
		{"owner/repo@4.2.1-0", true, true, false},
		{"owner/repo@4.2.1-01alpha", true, true, false},
		{"owner/repo@4.2.1+build.01", true, true, false},
		{"owner/repo@v4.2.1-beta.2+build.3", true, true, false},
		{"owner/repo@v04.2", false, false, false},
		{"owner/repo@v4.02", false, false, false},
		{"owner/repo@v4.2.01", false, false, false},
		{"owner/repo@4.2.1-01", false, false, false},
		{"owner/repo@4.2.1-rc..1", false, false, false},
		{"owner/repo@4.2.1-", false, false, false},
		{"owner/repo@4.2.1+", false, false, false},
		{"owner/repo@4.2.1+build..1", false, false, false},
		{"owner/repo@4.2.1-rc_1", false, false, false},
		{"owner/repo@v4.2-rc.1", false, false, false},
		{"owner/repo@4.2.1.0", false, false, false},
		{"owner/repo@ 4.2.1", false, false, false},
		{"owner/repo@4.2.1\n", false, false, false},
		{"owner/repo@main", false, false, false},
		{"owner/repo@release/v1", false, false, false},
		{"owner/repo@0123456", false, false, false},
		{"owner/repo@" + sha[:39], false, false, false},
		{"owner/repo@" + sha + "0", false, false, false},
		{"owner/repo@" + sha[:39] + "g", false, false, false},
		{"owner/repo@" + sha + "\n", false, false, false},
		{"owner/repo@@" + sha, false, false, false},
		{"owner/repo@", false, false, false},
		{"owner/repo", false, false, false},
		{"./local", true, true, true},
		{"${{ inputs.action }}", true, true, true},
		{"owner/repo@${{ inputs.ref }}", true, true, true},
		{"", true, true, true},
	}
	for _, level := range []ActionPinningLevel{ActionPinningMajorMinor, ActionPinningSemver, ActionPinningCommitSHA} {
		for _, job := range []bool{false, true} {
			for _, tc := range tests {
				t.Run(fmt.Sprintf("level=%d/job=%t/%s", level, job, tc.uses), func(t *testing.T) {
					r := NewRuleActionPinning()
					r.Level = level
					want := 1
					if (level == ActionPinningMajorMinor && tc.majorMinor) || (level == ActionPinningSemver && tc.semver) || (level == ActionPinningCommitSHA && tc.sha) {
						want = 0
					}
					u := &String{Value: tc.uses, Pos: &Pos{Line: 3, Col: 11}}
					var err error
					if job {
						u.Value = strings.Replace(u.Value, "owner/repo", "owner/repo/.github/workflows/ci.yml", 1)
						err = r.VisitJobPre(&Job{WorkflowCall: &WorkflowCall{Uses: u}})
					} else {
						err = r.VisitStep(&Step{Exec: &ExecAction{Uses: u}})
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(r.Errs()) != want {
						t.Fatalf("wanted %d errors but got %v", want, r.Errs())
					}
					if want > 0 {
						e := r.Errs()[0]
						if e.Kind != "action-pinning" || e.Line != 3 || e.Column != 11 || !strings.HasPrefix(e.Message, fmt.Sprintf("%q at \"uses\" must be pinned to ", u.Value)) {
							t.Fatalf("unexpected diagnostic: %v", e)
						}
					}
				})
			}
		}
	}

}

func TestRuleActionPinningDefaultLevel(t *testing.T) {
	r := NewRuleActionPinning()
	if r.Level != ActionPinningSemver {
		t.Fatalf("default level is %d, want semver", r.Level)
	}
}

func TestRuleActionPinningInvalidLevel(t *testing.T) {
	r := NewRuleActionPinning()
	r.Level = ActionPinningLevel(255)
	u := &String{Value: "owner/repo@0123456789abcdef0123456789abcdef01234567", Pos: &Pos{}}
	if err := r.VisitStep(&Step{Exec: &ExecAction{Uses: u}}); err != nil {
		t.Fatal(err)
	}
	if len(r.Errs()) != 1 || r.Errs()[0].Message != "invalid action pinning level 255" {
		t.Fatal("unexpected diagnostics:", r.Errs())
	}
}

func TestRuleActionPinningSkipOtherNodes(t *testing.T) {
	r := NewRuleActionPinning()
	for _, s := range []*Step{
		{},
		{Exec: &ExecRun{}},
		{Exec: &ExecAction{}},
		{Exec: &ExecAction{Uses: &String{Value: "docker://alpine:3"}}},
	} {
		if err := r.VisitStep(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, j := range []*Job{{}, {WorkflowCall: &WorkflowCall{}}} {
		if err := r.VisitJobPre(j); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.Errs()) != 0 {
		t.Fatal("unexpected errors:", r.Errs())
	}
}
