package actionlint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

func parseActionPinningLevel(value string) (ActionPinningLevel, error) {
	switch value {
	case "major-minor":
		return ActionPinningMajorMinor, nil
	case "semver":
		return ActionPinningSemver, nil
	case "commit-sha":
		return ActionPinningCommitSHA, nil
	default:
		return 0, fmt.Errorf("invalid action pinning level %q: expected major-minor, semver, or commit-sha", value)
	}
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (level *ActionPinningLevel) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
		return fmt.Errorf("action pinning level must be a string")
	}
	v, err := parseActionPinningLevel(n.Value)
	if err == nil {
		*level = v
	}
	return err
}

// ActionPinningLevel specifies the required precision of an action reference.
type ActionPinningLevel uint8

const (
	// ActionPinningSemver accepts semantic versions and full commit SHAs. It is the default.
	ActionPinningSemver ActionPinningLevel = iota
	// ActionPinningMajorMinor also accepts versions with only major and minor components.
	ActionPinningMajorMinor
	// ActionPinningCommitSHA accepts only full lowercase commit SHAs.
	ActionPinningCommitSHA
)

// RuleActionPinning checks that remote actions and reusable workflows use pinned references.
type RuleActionPinning struct {
	RuleBase
	// ActionPinningConfig specifies the level (SemVer by default) and pinning exemptions.
	ActionPinningConfig
}

// NewRuleActionPinning creates a new RuleActionPinning instance.
func NewRuleActionPinning() *RuleActionPinning {
	return &RuleActionPinning{
		RuleBase: RuleBase{
			name: "action-pinning",
			desc: "Checks that remote actions and reusable workflows use pinned references",
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

const (
	actionPinningNumber     = `(0|[1-9][0-9]*)`
	actionPinningPrerelease = `(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)`
)

var (
	reActionPinningSHA        = regexp.MustCompile(`\A[0-9a-f]{40}\z`)
	reActionPinningMajorMinor = regexp.MustCompile(`\Av?` + actionPinningNumber + `\.` + actionPinningNumber + `\z`)
	reActionPinningSemver     = regexp.MustCompile(`\Av?` + actionPinningNumber + `\.` + actionPinningNumber + `\.` + actionPinningNumber +
		`(-` + actionPinningPrerelease + `(\.` + actionPinningPrerelease + `)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?\z`)
)

func (rule *RuleActionPinning) checkUses(uses *String) {
	if uses == nil || uses.Value == "" || uses.ContainsExpression() || strings.HasPrefix(uses.Value, "./") {
		return
	}
	action, ref, _ := strings.Cut(uses.Value, "@")
	owner, rest, _ := strings.Cut(action, "/")
	repo, _, _ := strings.Cut(rest, "/")
	if repo != "" && rule.matches(rule.AllowedOwners, rule.AllowedActions, owner, repo) &&
		!rule.matches(rule.DeniedOwners, rule.DeniedActions, owner, repo) {
		return
	}
	valid := reActionPinningSHA.MatchString(ref)
	var requirement string
	switch rule.Level {
	case ActionPinningMajorMinor:
		valid = valid || reActionPinningMajorMinor.MatchString(ref) || reActionPinningSemver.MatchString(ref)
		requirement = "a major.minor version, a semantic version, or a full lowercase 40-character commit SHA"
	case ActionPinningSemver:
		valid = valid || reActionPinningSemver.MatchString(ref)
		requirement = "a semantic version or a full lowercase 40-character commit SHA"
	case ActionPinningCommitSHA:
		requirement = "a full lowercase 40-character commit SHA"
	default:
		rule.Errorf(uses.Pos, "invalid action pinning level %d", rule.Level)
		return
	}
	if !valid {
		rule.Errorf(uses.Pos, "%q at \"uses\" must be pinned to %s", uses.Value, requirement)
	}
}

func (rule *RuleActionPinning) matches(owners, actions []string, owner, repo string) bool {
	return slices.ContainsFunc(owners, func(entry string) bool { return strings.EqualFold(entry, owner) }) ||
		slices.ContainsFunc(actions, func(entry string) bool { return strings.EqualFold(entry, owner+"/"+repo) })
}
