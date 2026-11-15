package actionlint

import (
	"regexp"
	"strings"
)

// RuleActionPinning checks that remote actions and reusable workflows are pinned to full commit SHAs.
type RuleActionPinning struct {
	RuleBase
}

// NewRuleActionPinning creates a new RuleActionPinning instance.
func NewRuleActionPinning() *RuleActionPinning {
	return &RuleActionPinning{
		RuleBase: RuleBase{
			name: "action-pinning",
			desc: "Checks that remote actions and reusable workflows are pinned to full commit SHAs",
		},
	}
}

// VisitStep is callback when visiting Step node.
func (rule *RuleActionPinning) VisitStep(n *Step) error {
	if e, ok := n.Exec.(*ExecAction); ok {
		if e.Uses != nil && !strings.HasPrefix(e.Uses.Value, "docker://") {
			rule.checkUses(e.Uses)
		}
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleActionPinning) VisitJobPre(n *Job) error {
	if n.WorkflowCall != nil {
		rule.checkUses(n.WorkflowCall.Uses)
	}
	return nil
}

var reActionPinningSHA = regexp.MustCompile(`\A[0-9a-fA-F]{40}\z`)

func (rule *RuleActionPinning) checkUses(uses *String) {
	if uses == nil || uses.Value == "" || uses.ContainsExpression() || strings.HasPrefix(uses.Value, "./") {
		return
	}
	_, ref, _ := strings.Cut(uses.Value, "@")
	if !reActionPinningSHA.MatchString(ref) {
		rule.Errorf(uses.Pos, "%q at \"uses\" must be pinned to a full 40-character commit SHA", uses.Value)
	}
}
