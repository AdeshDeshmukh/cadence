package developer

import (
	"regexp"
	"strings"
)

// RiskLevel categorizes the potential danger or side-effects of a command.
type RiskLevel string

const (
	// RiskSafe represents read-only, non-destructive, inspection, or standard test commands.
	RiskSafe RiskLevel = "safe"
	// RiskRisky represents commands that alter state, commit changes, create branches, or install packages.
	RiskRisky RiskLevel = "risky"
	// RiskDestructive represents commands that delete data, overwrite history, terminate processes, or reformat disks.
	RiskDestructive RiskLevel = "destructive"
)

// Severity returns an integer ranking for risk comparison (Destructive > Risky > Safe).
func (r RiskLevel) Severity() int {
	switch r {
	case RiskDestructive:
		return 3
	case RiskRisky:
		return 2
	case RiskSafe:
		return 1
	default:
		return 2
	}
}

// RiskAssessment holds the full risk evaluation for a command.
type RiskAssessment struct {
	Level                RiskLevel `json:"level"`
	Reason               string    `json:"reason"`
	Rule                 string    `json:"rule"`
	RequiresConfirmation bool      `json:"requires_confirmation"`
	HighlightedTokens    []string  `json:"highlighted_tokens,omitempty"`
}

// ClassifyCommand parses and classifies a command string into Safe, Risky, or Destructive.
// Compound commands (chained via &&, ||, ;, |, &) are split and evaluated individually;
// the highest risk level among all sub-commands determines the overall classification.
func ClassifyCommand(cmdStr string) RiskAssessment {
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" {
		return RiskAssessment{
			Level:                RiskSafe,
			Reason:               "Empty command",
			Rule:                 "empty",
			RequiresConfirmation: false,
		}
	}

	subCmds := splitCompoundCommands(trimmed)
	if len(subCmds) == 0 {
		subCmds = []string{trimmed}
	}

	var highestRisk RiskAssessment
	highestRisk.Level = RiskSafe
	highestRisk.RequiresConfirmation = false

	for _, sub := range subCmds {
		assessment := evaluateSingleCommand(sub)
		if assessment.Level.Severity() > highestRisk.Level.Severity() {
			highestRisk = assessment
		} else if assessment.Level.Severity() == highestRisk.Level.Severity() && highestRisk.Rule == "" {
			highestRisk = assessment
		}
	}

	// If there were multiple commands, clarify in the reason if the highest is destructive/risky
	if len(subCmds) > 1 && highestRisk.Level != RiskSafe {
		highestRisk.Reason = highestRisk.Reason + " (part of compound command pipeline)"
	}

	return highestRisk
}

// splitCompoundCommands splits a pipeline or chain of shell commands.
func splitCompoundCommands(cmdStr string) []string {
	// Simple shell delimiter splitter recognizing &&, ||, ;, |, &
	// Avoiding splits inside double or single quotes.
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	runes := []rune(cmdStr)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			current.WriteRune(r)
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			current.WriteRune(r)
			continue
		}

		if !inSingle && !inDouble {
			if r == ';' || r == '|' || r == '&' {
				// Check for two-character operators: &&, ||
				if (r == '&' && i+1 < n && runes[i+1] == '&') || (r == '|' && i+1 < n && runes[i+1] == '|') {
					i++ // skip second char
				}
				seg := strings.TrimSpace(current.String())
				if seg != "" {
					parts = append(parts, seg)
				}
				current.Reset()
				continue
			}
		}

		current.WriteRune(r)
	}

	seg := strings.TrimSpace(current.String())
	if seg != "" {
		parts = append(parts, seg)
	}

	return parts
}

var (
	reSudoPrefix = regexp.MustCompile(`^(?:sudo|doas)\s+`)
	reEnvPrefix  = regexp.MustCompile(`^(?:env|nohup|time)\s+`)
)

func evaluateSingleCommand(cmd string) RiskAssessment {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return RiskAssessment{Level: RiskSafe, Reason: "Empty command", Rule: "empty"}
	}

	// Strip env / sudo prefixes but note privilege escalation
	isSudo := false
	for {
		if loc := reSudoPrefix.FindStringIndex(trimmed); loc != nil {
			isSudo = true
			trimmed = strings.TrimSpace(trimmed[loc[1]:])
			continue
		}
		if loc := reEnvPrefix.FindStringIndex(trimmed); loc != nil {
			trimmed = strings.TrimSpace(trimmed[loc[1]:])
			continue
		}
		break
	}

	tokens := tokenize(trimmed)
	if len(tokens) == 0 {
		return RiskAssessment{Level: RiskSafe, Reason: "Empty command", Rule: "empty"}
	}

	baseBin := strings.ToLower(tokens[0])

	// 1. Check Destructive rules first
	if assessment, matched := checkDestructiveRules(baseBin, tokens, trimmed); matched {
		if isSudo {
			assessment.Reason = "Superuser execution: " + assessment.Reason
			assessment.HighlightedTokens = append([]string{"sudo"}, assessment.HighlightedTokens...)
		}
		assessment.RequiresConfirmation = true
		return assessment
	}

	// 2. Check Risky rules
	if assessment, matched := checkRiskyRules(baseBin, tokens, trimmed); matched {
		if isSudo {
			assessment.Reason = "Superuser execution: " + assessment.Reason
			assessment.HighlightedTokens = append([]string{"sudo"}, assessment.HighlightedTokens...)
		}
		assessment.RequiresConfirmation = true
		return assessment
	}

	// 3. Check Safe rules
	if assessment, matched := checkSafeRules(baseBin, tokens, trimmed); matched {
		if isSudo {
			// Elevate safe commands run under sudo to risky
			return RiskAssessment{
				Level:                RiskRisky,
				Reason:               "Superuser privileges requested for inspection/command: " + assessment.Reason,
				Rule:                 "sudo-elevation",
				RequiresConfirmation: true,
				HighlightedTokens:    []string{"sudo"},
			}
		}
		assessment.RequiresConfirmation = false
		return assessment
	}

	// 4. Default fallback: unknown command
	level := RiskRisky
	reason := "Unrecognized command; requires user confirmation as a safety precaution"
	if isSudo {
		reason = "Superuser execution of unrecognized command"
	}
	return RiskAssessment{
		Level:                level,
		Reason:               reason,
		Rule:                 "unknown-command",
		RequiresConfirmation: true,
		HighlightedTokens:    tokens[:1],
	}
}

func checkDestructiveRules(baseBin string, tokens []string, fullCmd string) (RiskAssessment, bool) {
	lowerCmd := strings.ToLower(fullCmd)

	// Git destructive operations
	if baseBin == "git" && len(tokens) > 1 {
		sub := strings.ToLower(tokens[1])

		// git reset --hard
		if sub == "reset" && hasAnyToken(tokens[2:], "--hard") {
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Hard reset discards all uncommitted working tree and staged changes permanently",
				Rule:              "git-reset-hard",
				HighlightedTokens: []string{"git", "reset", "--hard"},
			}, true
		}

		// git push --force / -f
		if sub == "push" && (hasAnyToken(tokens[2:], "--force", "-f", "--force-with-lease") || strings.Contains(fullCmd, " +")) {
			token := "--force"
			if hasAnyToken(tokens[2:], "-f") {
				token = "-f"
			} else if hasAnyToken(tokens[2:], "--force-with-lease") {
				token = "--force-with-lease"
			}
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Force push rewrites remote commit history and may overwrite collaborators' commits",
				Rule:              "git-force-push",
				HighlightedTokens: []string{"git", "push", token},
			}, true
		}

		// git clean -f / -fd / -fx
		if sub == "clean" && (hasFlagPrefix(tokens[2:], "-f") || hasAnyToken(tokens[2:], "--force")) {
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Git clean deletes untracked files and directories permanently",
				Rule:              "git-clean",
				HighlightedTokens: []string{"git", "clean"},
			}, true
		}

		// git branch -D / --delete --force
		if sub == "branch" && (hasAnyToken(tokens[2:], "-D") || (hasAnyToken(tokens[2:], "-d", "--delete") && hasAnyToken(tokens[2:], "--force", "-f"))) {
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Force deleting branch permanently discards unmerged commits",
				Rule:              "git-branch-force-delete",
				HighlightedTokens: []string{"git", "branch", "-D"},
			}, true
		}

		// git restore . / git checkout -- . (without specific file or with dot / star)
		if (sub == "restore" || sub == "checkout") && (hasAnyToken(tokens[2:], ".", "*", "-f", "--force")) {
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Discarding working tree changes overwrites local edits permanently",
				Rule:              "git-discard-changes",
				HighlightedTokens: []string{"git", sub},
			}, true
		}

		// git prune
		if sub == "prune" {
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Prunes unreachable loose objects from the git repository",
				Rule:              "git-prune",
				HighlightedTokens: []string{"git", "prune"},
			}, true
		}
	}

	// Deletion / Unlink commands
	switch baseBin {
	case "rm":
		isRecursive := hasAnyToken(tokens[1:], "-r", "-R", "-rf", "-fr", "--recursive")
		isForce := hasAnyToken(tokens[1:], "-f", "-rf", "-fr", "--force")
		reason := "Deletes files permanently from the filesystem"
		rule := "rm-file"
		if isRecursive || isForce {
			reason = "Recursively and forcefully deletes directory trees permanently"
			rule = "rm-recursive"
		}
		return RiskAssessment{
			Level:             RiskDestructive,
			Reason:            reason,
			Rule:              rule,
			HighlightedTokens: tokens,
		}, true

	case "shred", "srm", "wipe", "unlink":
		return RiskAssessment{
			Level:             RiskDestructive,
			Reason:            "Permanently destroys and overwrites filesystem data",
			Rule:              "disk-wipe",
			HighlightedTokens: []string{baseBin},
		}, true
	}

	if baseBin == "dd" || baseBin == "wipefs" || baseBin == "fdisk" || baseBin == "gdisk" || baseBin == "parted" || baseBin == "mkfs" || strings.HasPrefix(baseBin, "mkfs.") {
		return RiskAssessment{
			Level:             RiskDestructive,
			Reason:            "Raw block device or filesystem partitioning/formatting operation",
			Rule:              "disk-raw-write",
			HighlightedTokens: []string{baseBin},
		}, true
	}

	switch baseBin {
	case "kill", "killall", "pkill":
		if hasAnyToken(tokens[1:], "-9", "-SIGKILL", "-KILL") || baseBin == "killall" || baseBin == "pkill" {
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Forcefully terminates running processes without allowing graceful shutdown",
				Rule:              "process-kill-force",
				HighlightedTokens: tokens,
			}, true
		}

	case "reboot", "shutdown", "poweroff", "halt", "init":
		return RiskAssessment{
			Level:             RiskDestructive,
			Reason:            "Reboots, powers down, or halts the operating system",
			Rule:              "system-shutdown",
			HighlightedTokens: []string{baseBin},
		}, true

	case "dropdb", "truncate":
		return RiskAssessment{
			Level:             RiskDestructive,
			Reason:            "Destroys database contents or truncates files",
			Rule:              "database-drop",
			HighlightedTokens: []string{baseBin},
		}, true
	}

	// Dangerous redirects targeting sensitive directories
	if strings.Contains(fullCmd, ">") {
		if strings.Contains(lowerCmd, "> /dev/") || strings.Contains(lowerCmd, "> /etc/") || strings.Contains(lowerCmd, "> /boot/") {
			return RiskAssessment{
				Level:             RiskDestructive,
				Reason:            "Redirects output directly into critical system or device files",
				Rule:              "dangerous-redirect",
				HighlightedTokens: []string{">"},
			}, true
		}
	}

	return RiskAssessment{}, false
}

func checkRiskyRules(baseBin string, tokens []string, fullCmd string) (RiskAssessment, bool) {
	// Git mutating operations
	if baseBin == "git" && len(tokens) > 1 {
		sub := strings.ToLower(tokens[1])
		switch sub {
		case "commit":
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Records staged changes to the repository commit history",
				Rule:              "git-commit",
				HighlightedTokens: []string{"git", "commit"},
			}, true
		case "push":
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Publishes local commits to a remote repository",
				Rule:              "git-push",
				HighlightedTokens: []string{"git", "push"},
			}, true
		case "pull":
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Fetches and integrates remote changes into the current branch",
				Rule:              "git-pull",
				HighlightedTokens: []string{"git", "pull"},
			}, true
		case "add":
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Updates the git index and stages file modifications",
				Rule:              "git-add",
				HighlightedTokens: []string{"git", "add"},
			}, true
		case "checkout", "switch":
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Switches branches or restores working trees",
				Rule:              "git-checkout",
				HighlightedTokens: []string{"git", sub},
			}, true
		case "branch":
			// If it's a branch creation or deletion (safe listing handled in checkSafeRules)
			if len(tokens) > 2 && !hasAnyToken(tokens[2:], "-a", "-r", "--list", "-l") {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Creates, modifies, or deletes a repository branch",
					Rule:              "git-branch-mutate",
					HighlightedTokens: []string{"git", "branch"},
				}, true
			}
		case "merge", "rebase", "cherry-pick", "revert":
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Modifies branch history and merges source trees",
				Rule:              "git-" + sub,
				HighlightedTokens: []string{"git", sub},
			}, true
		case "stash":
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Saves, applies, or modifies local stashed changes",
				Rule:              "git-stash",
				HighlightedTokens: []string{"git", "stash"},
			}, true
		case "tag":
			if len(tokens) > 2 && !hasAnyToken(tokens[2:], "-l", "--list") {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Creates, updates, or deletes git release tags",
					Rule:              "git-tag",
					HighlightedTokens: []string{"git", "tag"},
				}, true
			}
		}
	}

	// Filesystem modifications
	switch baseBin {
	case "mv":
		return RiskAssessment{
			Level:             RiskRisky,
			Reason:            "Moves or renames files and directories",
			Rule:              "fs-move",
			HighlightedTokens: []string{"mv"},
		}, true
	case "cp":
		return RiskAssessment{
			Level:             RiskRisky,
			Reason:            "Copies files or directories, potentially overwriting destination files",
			Rule:              "fs-copy",
			HighlightedTokens: []string{"cp"},
		}, true
	case "mkdir":
		return RiskAssessment{
			Level:             RiskRisky,
			Reason:            "Creates new directories on the filesystem",
			Rule:              "fs-mkdir",
			HighlightedTokens: []string{"mkdir"},
		}, true
	case "touch":
		return RiskAssessment{
			Level:             RiskRisky,
			Reason:            "Creates new empty files or updates file access timestamps",
			Rule:              "fs-touch",
			HighlightedTokens: []string{"touch"},
		}, true
	case "chmod", "chown", "chgrp":
		return RiskAssessment{
			Level:             RiskRisky,
			Reason:            "Modifies filesystem file permissions or ownership",
			Rule:              "fs-permissions",
			HighlightedTokens: []string{baseBin},
		}, true
	case "tar", "unzip", "gunzip":
		if hasAnyToken(tokens[1:], "-x", "-xf", "x", "extract") || baseBin != "tar" {
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Unpacks archive contents into the target directory",
				Rule:              "fs-extract",
				HighlightedTokens: []string{baseBin},
			}, true
		}
	case "sed":
		if hasAnyToken(tokens[1:], "-i", "--in-place") {
			return RiskAssessment{
				Level:             RiskRisky,
				Reason:            "Performs in-place file modifications without separate backup",
				Rule:              "sed-inplace",
				HighlightedTokens: []string{"sed", "-i"},
			}, true
		}
	}

	// Package managers (install / update / remove / tidy)
	switch baseBin {
	case "npm", "pnpm", "yarn", "bun":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "install" || sub == "i" || sub == "add" || sub == "update" || sub == "remove" || sub == "uninstall" {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Installs, updates, or removes packages and dependency lockfiles",
					Rule:              "package-manager-mutate",
					HighlightedTokens: []string{baseBin, sub},
				}, true
			}
		}
	case "go":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "get" || sub == "install" || (sub == "mod" && len(tokens) > 2 && (tokens[2] == "tidy" || tokens[2] == "download")) {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Fetches or modifies Go modules and dependencies",
					Rule:              "go-mod-mutate",
					HighlightedTokens: []string{"go", sub},
				}, true
			}
		}
	case "pip", "pip3", "poetry", "pipenv":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "install" || sub == "uninstall" || sub == "add" || sub == "remove" {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Installs or uninstalls Python packages and dependencies",
					Rule:              "python-pkg-mutate",
					HighlightedTokens: []string{baseBin, sub},
				}, true
			}
		}
	case "cargo":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "add" || sub == "install" || sub == "remove" || sub == "update" {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Modifies Rust dependencies or installs binaries",
					Rule:              "cargo-mutate",
					HighlightedTokens: []string{"cargo", sub},
				}, true
			}
		}
	case "brew", "apt", "apt-get":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "install" || sub == "uninstall" || sub == "upgrade" || sub == "remove" {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Installs or updates system software packages",
					Rule:              "sys-pkg-mutate",
					HighlightedTokens: []string{baseBin, sub},
				}, true
			}
		}
	}

	// Process signals / Service management
	switch baseBin {
	case "kill":
		return RiskAssessment{
			Level:             RiskRisky,
			Reason:            "Sends process termination signal",
			Rule:              "process-signal",
			HighlightedTokens: tokens,
		}, true
	case "systemctl", "service", "launchctl":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "start" || sub == "stop" || sub == "restart" || sub == "reload" || sub == "load" || sub == "unload" {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Controls system service or daemon process state",
					Rule:              "service-control",
					HighlightedTokens: []string{baseBin, sub},
				}, true
			}
		}
	case "docker":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "run" || sub == "stop" || sub == "rm" || sub == "restart" || sub == "build" {
				return RiskAssessment{
					Level:             RiskRisky,
					Reason:            "Modifies Docker containers or image state",
					Rule:              "docker-mutate",
					HighlightedTokens: []string{baseBin, sub},
				}, true
			}
		}
	}

	return RiskAssessment{}, false
}

func checkSafeRules(baseBin string, tokens []string, fullCmd string) (RiskAssessment, bool) {
	// Git inspection / status / read-only commands
	if baseBin == "git" && len(tokens) > 1 {
		sub := strings.ToLower(tokens[1])
		switch sub {
		case "status":
			return RiskAssessment{
				Level:             RiskSafe,
				Reason:            "Displays repository working tree status (read-only)",
				Rule:              "git-status",
				HighlightedTokens: []string{"git", "status"},
			}, true
		case "diff":
			return RiskAssessment{
				Level:             RiskSafe,
				Reason:            "Displays diff changes between commits, commit and working tree (read-only)",
				Rule:              "git-diff",
				HighlightedTokens: []string{"git", "diff"},
			}, true
		case "log":
			return RiskAssessment{
				Level:             RiskSafe,
				Reason:            "Displays commit history logs (read-only)",
				Rule:              "git-log",
				HighlightedTokens: []string{"git", "log"},
			}, true
		case "show":
			return RiskAssessment{
				Level:             RiskSafe,
				Reason:            "Shows various types of objects, diffs, or commits (read-only)",
				Rule:              "git-show",
				HighlightedTokens: []string{"git", "show"},
			}, true
		case "branch":
			if len(tokens) == 2 || hasAnyToken(tokens[2:], "-a", "-r", "--list", "-l") {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Lists local and remote branches (read-only)",
					Rule:              "git-branch-list",
					HighlightedTokens: []string{"git", "branch"},
				}, true
			}
		case "remote":
			if len(tokens) == 2 || hasAnyToken(tokens[2:], "-v", "--verbose") {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Lists configured remote repositories (read-only)",
					Rule:              "git-remote-list",
					HighlightedTokens: []string{"git", "remote"},
				}, true
			}
		case "check-ref-format", "rev-parse", "config":
			if sub != "config" || !hasAnyToken(tokens[2:], "--add", "--unset", "--set") {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Git inspection utility (read-only)",
					Rule:              "git-inspect",
					HighlightedTokens: []string{"git", sub},
				}, true
			}
		}
	}

	// Testing commands (Zero state change, verification)
	switch baseBin {
	case "go":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "test" || sub == "vet" || sub == "version" || sub == "doc" {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Runs Go test suite, linter, or documentation (read-only)",
					Rule:              "go-test",
					HighlightedTokens: []string{"go", sub},
				}, true
			}
			if sub == "build" {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Compiles Go packages into local binary",
					Rule:              "go-build",
					HighlightedTokens: []string{"go", "build"},
				}, true
			}
		}
	case "pytest", "jest", "vitest", "cargo-test":
		return RiskAssessment{
			Level:             RiskSafe,
			Reason:            "Executes automated unit/integration test runner",
			Rule:              "test-runner",
			HighlightedTokens: []string{baseBin},
		}, true
	case "cargo":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "test" || sub == "check" || sub == "clippy" {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Runs Cargo tests, type check, or clippy linter",
					Rule:              "cargo-test",
					HighlightedTokens: []string{"cargo", sub},
				}, true
			}
			if sub == "build" {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Compiles Rust project into target binary",
					Rule:              "cargo-build",
					HighlightedTokens: []string{"cargo", "build"},
				}, true
			}
		}
	case "npm", "pnpm", "yarn", "bun":
		if len(tokens) > 1 {
			sub := strings.ToLower(tokens[1])
			if sub == "test" || (sub == "run" && len(tokens) > 2 && (tokens[2] == "test" || tokens[2] == "lint" || tokens[2] == "typecheck")) {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Executes JavaScript/TypeScript test or lint runner",
					Rule:              "node-test",
					HighlightedTokens: []string{baseBin, sub},
				}, true
			}
			if sub == "list" || sub == "outdated" {
				return RiskAssessment{
					Level:             RiskSafe,
					Reason:            "Lists installed package dependencies (read-only)",
					Rule:              "node-list",
					HighlightedTokens: []string{baseBin, sub},
				}, true
			}
		}
	case "make":
		if len(tokens) > 1 && strings.Contains(strings.ToLower(tokens[1]), "test") {
			return RiskAssessment{
				Level:             RiskSafe,
				Reason:            "Runs Makefile test targets",
				Rule:              "make-test",
				HighlightedTokens: []string{"make", tokens[1]},
			}, true
		}
	}

	// Linters and static analysis
	switch baseBin {
	case "golangci-lint", "eslint", "ruff", "mypy", "flake8", "tsc":
		return RiskAssessment{
			Level:             RiskSafe,
			Reason:            "Runs code analysis and linting checks",
			Rule:              "linter",
			HighlightedTokens: []string{baseBin},
		}, true
	}

	// Read-only filesystem and system inspection commands
	switch baseBin {
	case "ls", "pwd", "cat", "head", "tail", "less", "more", "grep", "egrep", "rg", "ag",
		"find", "stat", "file", "wc", "echo", "printf", "which", "whereis", "type",
		"ps", "top", "htop", "df", "du", "uptime", "whoami", "uname", "env", "printenv",
		"diff", "curl", "ping", "man", "info", "help":
		return RiskAssessment{
			Level:             RiskSafe,
			Reason:            "Standard system inspection or read-only utility",
			Rule:              "inspection-util",
			HighlightedTokens: []string{baseBin},
		}, true
	}

	return RiskAssessment{}, false
}

// tokenize splits a command string into arguments while respecting basic quotes.
func tokenize(cmdStr string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for _, r := range cmdStr {
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func hasAnyToken(tokens []string, targets ...string) bool {
	targetMap := make(map[string]bool, len(targets))
	for _, t := range targets {
		targetMap[strings.ToLower(t)] = true
	}
	for _, token := range tokens {
		if targetMap[strings.ToLower(token)] {
			return true
		}
	}
	return false
}

func hasFlagPrefix(tokens []string, prefix string) bool {
	prefix = strings.ToLower(prefix)
	for _, token := range tokens {
		if strings.HasPrefix(strings.ToLower(token), prefix) {
			return true
		}
	}
	return false
}
