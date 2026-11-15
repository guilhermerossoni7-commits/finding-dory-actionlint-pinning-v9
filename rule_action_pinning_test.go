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
		{"./local@${{ inputs.ref }}", true, true, true},
		{"docker://alpine:latest", true, true, true},
		{"docker://alpine:${{ inputs.tag }}", true, true, true},
		{"${{ inputs.action }}", true, true, true},
		{"${{ inputs.owner }}/repo@main", true, true, true},
		{"owner/${{ inputs.repo }}@main", true, true, true},
		{"${{ 'owner/repo@main' }}", true, true, true},
		{"owner/repo@${{ inputs.ref }}", false, false, false},
		{"owner/repo@v1.${{ inputs.minor }}.0", false, false, false},
		{"owner/repo@${{ 'v1.2.3@other' }}", false, false, false},
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
						kind := "action"
						if job {
							kind = "reusable workflow"
						}
						prefix := fmt.Sprintf("%s %q at \"uses\" must be pinned to ", kind, u.Value)
						if u.ContainsExpression() {
							prefix = fmt.Sprintf("cannot verify pinning of %s %q at \"uses\" because its version is a dynamic expression", kind, u.Value)
						}
						if e.Kind != "action-pinning" || e.Line != 3 || e.Column != 11 || !strings.HasPrefix(e.Message, prefix) {
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

func TestRuleActionPinningLists(t *testing.T) {
	for _, tc := range []struct {
		config, uses string
		want         int
	}{
		{"allowed-owners: [OWNER]", "owner/repo@main", 0},
		{"allowed-owners: [owner]", "OWNER/repo@main", 0},
		{"allowed-owners: [owner]", "owner/repo@${{ inputs.ref }}", 0},
		{"allowed-owners: [owner]\ndenied-actions: [owner/repo]", "owner/repo@${{ inputs.ref }}", 1},
		{"allowed-owners: [other]", "owner/repo@main", 1},
		{"allowed-actions: [OWNER/REPO]", "owner/repo@main", 0},
		{"allowed-actions: [owner/repo]", "owner/repo/subaction@main", 0},
		{"allowed-actions: [owner/repo]", "owner/another@main", 1},
		{"allowed-actions: [owner/repo]", "owner/repository@main", 1},
		{"allowed-owners: [owner]\ndenied-owners: [OWNER]", "owner/repo@main", 1},
		{"allowed-owners: [owner]\ndenied-actions: [OWNER/REPO]", "owner/repo@main", 1},
		{"allowed-actions: [owner/repo]\ndenied-owners: [OWNER]", "owner/repo@main", 1},
		{"allowed-actions: [owner/repo]\ndenied-actions: [OWNER/REPO]", "owner/repo@main", 1},
		{"allowed-owners: [owner]\ndenied-actions: [owner/another]", "owner/repo@main", 0},
		{"denied-owners: [OWNER]", "owner/repo@0123456789abcdef0123456789abcdef01234567", 0},
		{"denied-actions: [OWNER/REPO]", "owner/repo@0123456789abcdef0123456789abcdef01234567", 0},
		{"allowed-owners: [owner]\ndenied-actions: [owner/repo]", "owner/repo@0123456789abcdef0123456789abcdef01234567", 0},
	} {
		for _, job := range []bool{false, true} {
			t.Run(fmt.Sprintf("job=%t/%s/%s", job, tc.config, tc.uses), func(t *testing.T) {
				cfg, err := ParseConfig([]byte("action-pinning:\n  " + strings.ReplaceAll(tc.config, "\n", "\n  ")))
				if err != nil {
					t.Fatal(err)
				}
				r := NewRuleActionPinning()
				r.ActionPinningConfig = *cfg.ActionPinning
				r.Level = ActionPinningCommitSHA
				u := &String{Value: tc.uses, Pos: &Pos{}}
				if job {
					u.Value = strings.Replace(u.Value, "@", "/.github/workflows/ci.yml@", 1)
					err = r.VisitJobPre(&Job{WorkflowCall: &WorkflowCall{Uses: u}})
				} else {
					err = r.VisitStep(&Step{Exec: &ExecAction{Uses: u}})
				}
				if err != nil || len(r.Errs()) != tc.want {
					t.Fatalf("got errors=%v err=%v; want %d diagnostics", r.Errs(), err, tc.want)
				}
			})
		}
	}
}

func TestRuleActionPinningSuggestions(t *testing.T) {
	for _, tc := range []struct {
		uses       string
		level      ActionPinningLevel
		suggestion string
	}{
		{"actions/add-to-project@main", ActionPinningSemver, "actions/add-to-project@v1.0.1"},
		{"ACTIONS/ADD-TO-PROJECT@main", ActionPinningMajorMinor, "actions/add-to-project@v1.0.1"},
		{"SamKirkland/FTP-Deploy-Action@v4", ActionPinningSemver, "SamKirkland/FTP-Deploy-Action@v4.3.6"},
		{"actions/add-to-project@${{ inputs.ref }}", ActionPinningSemver, "actions/add-to-project@v1.0.1"},
		{"actions/add-to-project@main", ActionPinningCommitSHA, ""},
		{"actions/checkout@v6", ActionPinningSemver, ""},
		{"actions/add-to-project/subaction@main", ActionPinningSemver, ""},
		{"owner/unknown@main", ActionPinningSemver, ""},
	} {
		t.Run(fmt.Sprintf("level=%d/%s", tc.level, tc.uses), func(t *testing.T) {
			for i := 0; i < 3; i++ {
				r := NewRuleActionPinning()
				r.Level = tc.level
				err := r.VisitStep(&Step{Exec: &ExecAction{Uses: &String{Value: tc.uses, Pos: &Pos{}}}})
				if err != nil || len(r.Errs()) != 1 {
					t.Fatalf("got errors=%v err=%v", r.Errs(), err)
				}
				message := r.Errs()[0].Message
				if tc.suggestion == "" {
					if strings.Contains(message, "for example") {
						t.Fatal("unexpected suggestion:", message)
					}
				} else if !strings.HasSuffix(message, fmt.Sprintf("; for example, use %q", tc.suggestion)) {
					t.Fatal("missing known version suggestion:", message)
				}
			}
		})
	}
}
