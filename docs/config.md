Configuration
=============

This document describes how to configure [actionlint](..) behavior.

Note that configuration file is optional. The author tries to keep configuration file as minimal as possible not to
bother users to configure behavior of actionlint. Running actionlint without configuration file would work fine in most
cases.

## Configuration file

Configuration file `actionlint.yaml` or `actionlint.yml` can be put in `.github` directory.

Note: If you're using [Super-Linter][], the file should be placed in a different directory. Please check the project's document.

```yaml
# Configuration related to self-hosted runner.
self-hosted-runner:
  # Labels of self-hosted runner in array of strings.
  labels:
    - linux.2xlarge
    - windows-latest-xl
    - linux-multi-gpu

# Configuration variables in array of strings defined in your repository or organization.
config-variables:
  - DEFAULT_RUNNER
  - JOB_NAME
  - ENVIRONMENT_STAGE

# null (or omission) disables action pinning. {} enables it with SemVer.
action-pinning: {}

# Path-specific configurations.
paths:
  # Glob pattern relative to the repository root for matching files. The path separator is always '/'.
  # This example configures any YAML file under the '.github/workflows/' directory.
  .github/workflows/**/*.{yml,yaml}:
    # List of regular expressions to filter errors by the error messages.
    ignore:
      # Ignore the specific error from shellcheck
      - 'shellcheck reported issue in this script: SC2086:.+'
  # This pattern only matches '.github/workflows/release.yaml' file.
  .github/workflows/release.yaml:
    # Require a full lowercase commit SHA for actions and reusable workflows.
    action-pinning:
      level: commit-sha
    ignore:
      # Ignore errors from the old runner check. This may be useful for (outdated) self-hosted runner environment.
      - 'the runner of ".+" action is too old to run on GitHub Actions'
```

- `self-hosted-runner`: Configuration for your self-hosted runner environment.
  - `labels`: Label names added to your self-hosted runners as list of pattern. Glob syntax supported by [`path.Match`][pat]
    is available.
- `config-variables`: [Configuration variables][vars]. When an array is set, actionlint will check `vars` properties strictly.
  An empty array means no variable is allowed. The default value `null` disables the check.
- `paths`: Configurations for specific file path patterns. This is a mapping from a glob pattern and the corresponding
  configuration.
  - `{glob}`: A file path glob pattern to apply the configuration. The path separator is always '/'. It is matched to the
    relative path from the repository root. For example `.github/workflows/**/*.yaml` matches all the workflow files (with
    `.yaml` file extension). For the glob syntax, please read the [doublestar][] library's documentation.
    - `ignore`: The configuration to ignore (filter) the errors by the error messages. This is an array of regular
      expressions. When one of the patterns matches the error message, the error will be ignored. It's similar to the
      `-ignore` command line option.
    - `action-pinning`: Overrides the global action pinning configuration. `null` disables the rule, `{}` enables it
      with the default `semver` level, and `{level: major-minor}` or `{level: commit-sha}` selects another level.
      A path can enable the rule even when it is disabled globally. Omitting this key inherits the global setting.

## Action pinning

The optional `action-pinning` mapping checks `uses` in steps and reusable workflow jobs. It is disabled when omitted
or set to `null`. An empty mapping (`{}`) enables the default `semver` level. The `level` field accepts:

| Level | Accepted references |
| --- | --- |
| `major-minor` | Major and minor versions such as `v1.2`, plus everything accepted by `semver` |
| `semver` | Major, minor, and patch versions such as `v1.2.3`, including pre-release and build metadata, plus full commit SHAs |
| `commit-sha` | Exactly 40 lowercase hexadecimal characters |

Version references may have an optional `v` prefix. References starting with `./` or `docker://` are skipped. A dynamic
action or workflow name is also skipped, but a static name with a dynamic version reports that pinning cannot be verified
(unless an allowed entry exempts it). Diagnostics distinguish actions from reusable workflows. For actions, a known
reference from the bundled `PopularActions` dataset is suggested when it satisfies the selected level. The suggestion is
an example from that dataset, not necessarily the latest release; versions and commit SHAs are not invented.
When multiple path patterns match, they are applied in alphabetical order; the last explicit `action-pinning` setting
determines whether the rule is enabled and which level is used. Path settings override the global setting; filter lists
are combined as described below.

The `-action-pinning-level` flag enables the rule and overrides only its level, taking precedence over global and path
settings, including `null`. Other settings, such as error ignore patterns, still apply:

```sh
actionlint -action-pinning-level commit-sha
```

The `allowed-owners`, `allowed-actions`, `denied-owners`, and `denied-actions` lists control exemptions from pinning:

```yaml
action-pinning:
  level: commit-sha
  allowed-owners: [trusted-org]
  allowed-actions: [another-org/helper]
  denied-owners: [strict-org]
  denied-actions: [trusted-org/deploy]
paths:
  .github/workflows/release.yaml:
    action-pinning:
      level: commit-sha
      allowed-actions: [release-org/helper]
      denied-actions: [another-org/helper]
```

Each list is the union of its global entries and entries from every matching path; an empty list does not remove inherited
entries. Owners and `owner/repo` names are matched without case sensitivity. Owner entries cannot contain slashes; action
entries must be exactly `owner/repo`, without a ref or subpath. They also match subactions and reusable workflows in that
repository. Entries are literal names, not glob patterns.

An allowed entry skips pinning unless any denied owner or action also matches. A denied entry only restores the normal
pinning check: it does not block an action whose reference satisfies the selected level. The command line level override
preserves all four lists, including when it re-enables a rule disabled by a path's `null` setting.

## Generate the initial configuration

You don't need to write the first configuration file by your hand. `actionlint` command can generate a default configuration
with `-init-config` flag.

```sh
actionlint -init-config
vim .github/actionlint.yaml
```

---

[Checks](checks.md) | [Installation](install.md) | [Usage](usage.md) | [Go API](api.md) | [References](reference.md)

[Super-Linter]: https://github.com/super-linter/super-linter
[pat]: https://pkg.go.dev/path#Match
[vars]: https://docs.github.com/en/actions/learn-github-actions/variables
[doublestar]: https://github.com/bmatcuk/doublestar
