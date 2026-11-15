package actionlint

import (
	"fmt"
	"strings"
	"testing"
)

func TestRuleActionPinning(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		uses string
		want int
	}{
		{"owner/repo@" + sha, 0},
		{"owner/repo/path@" + sha, 0},
		{"owner/repo@" + strings.ToUpper(sha), 0},
		{"owner/repo@v4", 1},
		{"owner/repo@v4.2.1", 1},
		{"owner/repo@main", 1},
		{"owner/repo@release/v1", 1},
		{"owner/repo@0123456", 1},
		{"owner/repo@" + sha[:39], 1},
		{"owner/repo@" + sha + "0", 1},
		{"owner/repo@" + sha[:39] + "g", 1},
		{"owner/repo@" + sha + "\n", 1},
		{"owner/repo@@" + sha, 1},
		{"owner/repo@", 1},
		{"owner/repo", 1},
		{"./local", 0},
		{"${{ inputs.action }}", 0},
		{"owner/repo@${{ inputs.ref }}", 0},
		{"", 0},
	}
	for _, job := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("job=%t/%s", job, tc.uses), func(t *testing.T) {
				r := NewRuleActionPinning()
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
				if len(r.Errs()) != tc.want {
					t.Fatalf("wanted %d errors but got %v", tc.want, r.Errs())
				}
				if tc.want > 0 {
					e := r.Errs()[0]
					want := fmt.Sprintf("%q at \"uses\" must be pinned to a full 40-character commit SHA", u.Value)
					if e.Kind != "action-pinning" || e.Line != 3 || e.Column != 11 || e.Message != want {
						t.Fatalf("unexpected diagnostic: %v", e)
					}
				}
			})
		}
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
