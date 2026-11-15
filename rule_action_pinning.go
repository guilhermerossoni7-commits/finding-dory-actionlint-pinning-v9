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
		rule.checkUses(e.Uses, "action")
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleActionPinning) VisitJobPre(n *Job) error {
	if n.WorkflowCall != nil {
		rule.checkUses(n.WorkflowCall.Uses, "reusable workflow")
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

func (rule *RuleActionPinning) checkUses(uses *String, kind string) {
	if uses == nil || uses.Value == "" || strings.HasPrefix(uses.Value, "./") || strings.HasPrefix(uses.Value, "docker://") {
		return
	}
	action, ref, _ := strings.Cut(uses.Value, "@")
	if strings.Contains(action, "${{") && uses.ContainsExpression() {
		return
	}
	owner, rest, _ := strings.Cut(action, "/")
	repo, _, _ := strings.Cut(rest, "/")
	if repo != "" && rule.matches(rule.AllowedOwners, rule.AllowedActions, owner, repo) &&
		!rule.matches(rule.DeniedOwners, rule.DeniedActions, owner, repo) {
		return
	}
	var requirement string
	switch rule.Level {
	case ActionPinningMajorMinor:
		requirement = "a major.minor version, a semantic version, or a full lowercase 40-character commit SHA"
	case ActionPinningSemver:
		requirement = "a semantic version or a full lowercase 40-character commit SHA"
	case ActionPinningCommitSHA:
		requirement = "a full lowercase 40-character commit SHA"
	default:
		rule.Errorf(uses.Pos, "invalid action pinning level %d", rule.Level)
		return
	}
	if rule.isPinned(ref) {
		return
	}
	message := fmt.Sprintf("%s %q at \"uses\" must be pinned to %s", kind, uses.Value, requirement)
	if ContainsExpression(ref) {
		message = fmt.Sprintf("cannot verify pinning of %s %q at \"uses\" because its version is a dynamic expression", kind, uses.Value)
	}
	if kind == "action" {
		if suggestion := rule.suggestion(action); suggestion != "" {
			message += fmt.Sprintf("; for example, use %q", suggestion)
		}
	}
	rule.Error(uses.Pos, message)
}

func (rule *RuleActionPinning) isPinned(ref string) bool {
	switch rule.Level {
	case ActionPinningMajorMinor:
		return reActionPinningSHA.MatchString(ref) || reActionPinningSemver.MatchString(ref) || reActionPinningMajorMinor.MatchString(ref)
	case ActionPinningSemver:
		return reActionPinningSHA.MatchString(ref) || reActionPinningSemver.MatchString(ref)
	case ActionPinningCommitSHA:
		return reActionPinningSHA.MatchString(ref)
	default:
		return false
	}
}

func (rule *RuleActionPinning) suggestion(action string) string {
	var suggestion string
	for spec := range PopularActions {
		name, ref, _ := strings.Cut(spec, "@")
		if strings.EqualFold(name, action) && rule.isPinned(ref) && (suggestion == "" || spec < suggestion) {
			suggestion = spec
		}
	}
	return suggestion
}

func (rule *RuleActionPinning) matches(owners, actions []string, owner, repo string) bool {
	return slices.ContainsFunc(owners, func(entry string) bool { return strings.EqualFold(entry, owner) }) ||
		slices.ContainsFunc(actions, func(entry string) bool { return strings.EqualFold(entry, owner+"/"+repo) })
}
