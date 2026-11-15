package actionlint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v4"
)

// IgnorePatterns is a list of regular expressions. These patterns are used for filtering errors by
// matching the error messages.
type IgnorePatterns []*regexp.Regexp

// Match returns whether the given error should be ignored due to the "ignore" configuration.
func (pats IgnorePatterns) Match(err *Error) bool {
	for _, r := range pats {
		if r.MatchString(err.Message) {
			return true
		}
	}
	return false
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (pats *IgnorePatterns) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("yaml: \"ignore\" must be a sequence node at line:%d,col:%d", n.Line, n.Column)
	}
	rs := make([]*regexp.Regexp, 0, len(n.Content))
	for _, p := range n.Content {
		r, err := regexp.Compile(p.Value)
		if err != nil {
			return fmt.Errorf("invalid regular expression %q in \"ignore\" at line%d,col:%d: %w", p.Value, n.Line, n.Column, err)
		}
		rs = append(rs, r)
	}
	*pats = rs
	return nil
}

// ActionPinningConfig enables action pinning with the specified level. An empty object uses SemVer.
type ActionPinningConfig struct {
	Level ActionPinningLevel `yaml:"level"`
	// Allowed entries skip pinning unless a denied entry also matches.
	AllowedOwners  []string `yaml:"allowed-owners"`
	AllowedActions []string `yaml:"allowed-actions"`
	DeniedOwners   []string `yaml:"denied-owners"`
	DeniedActions  []string `yaml:"denied-actions"`
}

var (
	reActionPinningOwner  = regexp.MustCompile(`\A[A-Za-z0-9-]+\z`)
	reActionPinningAction = regexp.MustCompile(`\A[A-Za-z0-9-]+/[A-Za-z0-9_.-]+\z`)
)

// UnmarshalYAML validates owner and repository filters when reading configuration.
func (cfg *ActionPinningConfig) UnmarshalYAML(n *yaml.Node) error {
	type plain ActionPinningConfig
	var value plain
	if err := n.Decode(&value); err != nil {
		return err
	}
	for _, list := range []struct {
		name    string
		entries []string
		pattern *regexp.Regexp
		format  string
	}{
		{"allowed-owners", value.AllowedOwners, reActionPinningOwner, "an owner without slashes"},
		{"allowed-actions", value.AllowedActions, reActionPinningAction, "owner/repo"},
		{"denied-owners", value.DeniedOwners, reActionPinningOwner, "an owner without slashes"},
		{"denied-actions", value.DeniedActions, reActionPinningAction, "owner/repo"},
	} {
		for _, entry := range list.entries {
			if !list.pattern.MatchString(entry) {
				return fmt.Errorf("invalid entry %q in action-pinning.%s: expected %s", entry, list.name, list.format)
			}
		}
	}
	*cfg = ActionPinningConfig(value)
	return nil
}

func (cfg *ActionPinningConfig) merge(other *ActionPinningConfig) {
	cfg.Level = other.Level
	cfg.AllowedOwners = append(cfg.AllowedOwners, other.AllowedOwners...)
	cfg.AllowedActions = append(cfg.AllowedActions, other.AllowedActions...)
	cfg.DeniedOwners = append(cfg.DeniedOwners, other.DeniedOwners...)
	cfg.DeniedActions = append(cfg.DeniedActions, other.DeniedActions...)
}

// PathConfig is a configuration for specific file path pattern. This is for values of the "paths" mapping
// in the configuration file.
type PathConfig struct {
	// Ignore is a list of patterns. They are used for ignoring errors by matching to the error messages.
	// It is similar to the "-ignore" command line option.
	Ignore IgnorePatterns `yaml:"ignore"`
	// ActionPinning overrides the global rule configuration for matching paths.
	ActionPinning    *ActionPinningConfig `yaml:"action-pinning"`
	actionPinningSet bool
}

// UnmarshalYAML preserves the difference between an absent override and explicit null.
func (cfg *PathConfig) UnmarshalYAML(n *yaml.Node) error {
	type plain PathConfig
	var value plain
	if err := n.Decode(&value); err != nil {
		return err
	}
	*cfg = PathConfig(value)
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == "action-pinning" {
			cfg.actionPinningSet = true
		}
	}
	return nil
}

// Config is configuration of actionlint. This struct instance is parsed from "actionlint.yaml"
// file usually put in ".github" directory.
type Config struct {
	// SelfHostedRunner is configuration for self-hosted runner.
	SelfHostedRunner struct {
		// Labels is label names for self-hosted runner.
		Labels []string `yaml:"labels"`
	} `yaml:"self-hosted-runner"`
	// ConfigVariables is names of configuration variables used in the checked workflows. When this value is nil,
	// property names of `vars` context will not be checked. Otherwise actionlint will report a name which is not
	// listed here as undefined config variables.
	// https://docs.github.com/en/actions/learn-github-actions/variables
	ConfigVariables []string `yaml:"config-variables"`
	// Paths is a "paths" mapping in the configuration file. The keys are glob patterns to match file paths.
	// And the values are corresponding configurations applied to the file paths.
	Paths map[string]PathConfig `yaml:"paths"`
	// ActionPinning is disabled when nil; an empty object enables the SemVer level.
	ActionPinning *ActionPinningConfig `yaml:"action-pinning"`
}

// PathConfigs returns a list of all PathConfig values matching to the given file path. The path must
// be relative to the root of the project.
func (cfg *Config) PathConfigs(path string) []PathConfig {
	path = filepath.ToSlash(path)

	var ret []PathConfig
	if cfg != nil {
		keys := make([]string, 0, len(cfg.Paths))
		for p := range cfg.Paths {
			keys = append(keys, p)
		}
		slices.Sort(keys)
		for _, p := range keys {
			// Glob patterns were validated in `ParseConfig()`
			if doublestar.MatchUnvalidated(p, path) {
				ret = append(ret, cfg.Paths[p])
			}
		}
	}
	return ret
}

// ParseConfig parses the given bytes as an actionlint config file. When deserializing the YAML file
// or the config validation fails, this function returns an error.
func ParseConfig(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		msg := strings.ReplaceAll(err.Error(), "\n", " ")
		return nil, errors.New(msg)
	}
	for pat := range c.Paths {
		if !doublestar.ValidatePattern(pat) {
			return nil, fmt.Errorf("invalid glob pattern %q in \"paths\"", pat)
		}
	}
	return &c, nil
}

// ReadConfigFile reads actionlint config file (actionlint.yaml) from the given file path.
func ReadConfigFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %q: %w", path, err)
	}
	c, err := ParseConfig(b)
	if err != nil {
		return nil, fmt.Errorf("could not parse config file %q: %w", path, err)
	}
	return c, nil
}

// loadRepoConfig reads config file from the repository's .github/actionlint.yml or
// .github/actionlint.yaml.
func loadRepoConfig(root string) (*Config, error) {
	for _, f := range []string{"actionlint.yaml", "actionlint.yml"} {
		p := filepath.Join(root, ".github", f)
		c, err := ReadConfigFile(p)
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue
		case err != nil:
			return nil, fmt.Errorf("could not parse config file %q: %w", p, err)
		default:
			return c, nil
		}
	}
	return nil, nil
}

func writeDefaultConfigFile(path string) error {
	b := []byte(`self-hosted-runner:
  # Labels of self-hosted runner in array of strings.
  labels: []

# Configuration variables in array of strings defined in your repository or
# organization. ` + "`null`" + ` means disabling configuration variables check.
# Empty array means no configuration variable is allowed.
config-variables: null

# null disables action pinning. {} enables it with the semver level.
# Available levels: major-minor, semver, commit-sha.
# allowed-owners and allowed-actions skip pinning; denied-owners and denied-actions
# take precedence and restore pinning checks. Lists are combined across paths.
action-pinning: null

# Configuration for file paths. The keys are glob patterns to match to file
# paths relative to the repository root. The values are the configurations for
# the file paths. Note that the path separator is always '/'.
# The following configurations are available.
#
# "ignore" is an array of regular expression patterns. Matched error messages
# are ignored. This is similar to the "-ignore" command line option.
paths:
#  .github/workflows/**/*.yml:
#    ignore: []
#    action-pinning: {level: commit-sha}
`)
	if err := os.WriteFile(path, b, 0644); err != nil {
		return fmt.Errorf("could not write default configuration file at %q: %w", path, err)
	}
	return nil
}
