package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type cliHelpTopic struct {
	Key       string
	Summary   string
	Usage     []string
	Examples  []string
	Notes     []string
	SeeAlso   []string
	Hidden    bool
	SortOrder int
}

var cliHelpTopics = map[string]cliHelpTopic{
	"roots": {
		Key:       "roots",
		Summary:   "Understand the public Agent Hub roots and path model.",
		Usage:     []string{"agenthub ls", "agenthub read profile/preferences", "agenthub read /project/demo"},
		Examples:  []string{"agenthub ls", "agenthub ls project", "agenthub read skill/writer/SKILL.md", "agenthub read secret/auth.github"},
		Notes:     []string{"Public roots are `profile`, `memory`, `project`, `skill`, `secret`, and `platform`.", "A leading `/` is optional. `project/demo` and `/project/demo` are equivalent.", "`project/<name>` is a summary view. Nested files live under paths like `project/demo/docs/notes.md`.", "`secret` is read-only in the current public command surface."},
		SeeAlso:   []string{"ls", "read", "write", "search"},
		SortOrder: 10,
	},
	"ls": {
		Key:       "ls",
		Summary:   "Browse the public Agent Hub roots or a subtree under them.",
		Usage:     []string{"agenthub ls [path]"},
		Examples:  []string{"agenthub ls", "agenthub ls /", "agenthub ls profile", "agenthub ls project/demo", "agenthub ls skill/writer"},
		Notes:     []string{"Use `agenthub ls` to discover the public roots first.", "Directory output uses paths relative to the Hub root.", "A leading `/` is optional."},
		SeeAlso:   []string{"roots", "read", "search"},
		SortOrder: 20,
	},
	"read": {
		Key:       "read",
		Summary:   "Read one Agent Hub path as text, a summary view, or a secret value.",
		Usage:     []string{"agenthub read <path> [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"},
		Examples:  []string{"agenthub read profile/preferences", "agenthub read project/demo", "agenthub read project/demo/docs/notes.md", "agenthub read skill/writer/SKILL.md", "agenthub read secret/auth.github"},
		Notes:     []string{"`project/<name>` returns the project summary and recent logs.", "Binary files are rejected instead of printing empty output.", "Use `--output FILE` when you want the final rendered result written locally."},
		SeeAlso:   []string{"ls", "write", "roots"},
		SortOrder: 30,
	},
	"write": {
		Key:       "write",
		Summary:   "Create or update Hub content from literal text, stdin, or a local file path.",
		Usage:     []string{"agenthub write <path> <content-or-file> [--literal] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"},
		Examples:  []string{"agenthub write profile/preferences ./preferences.md", "agenthub write memory \"Remember this\"", "agenthub write project/demo/docs/notes.md ./notes.md", "agenthub write skill/writer/SKILL.md -"},
		Notes:     []string{"The second argument may be literal text, `-` for stdin, or a local file path.", "Use `--literal` when an argument that looks like a path should stay plain text.", "`memory` writes a new scratch memory item instead of overwriting a fixed file.", "`secret` is intentionally read-only in the current public CLI."},
		SeeAlso:   []string{"read", "log", "import"},
		SortOrder: 40,
	},
	"search": {
		Key:       "search",
		Summary:   "Search Hub content globally or under one public path scope.",
		Usage:     []string{"agenthub search <query> [path] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"},
		Examples:  []string{"agenthub search migration", "agenthub search \"memory marker\" memory", "agenthub search \"launch checklist\" project/demo"},
		Notes:     []string{"When the optional path is omitted, search runs across the public Hub roots.", "`secret` search is not part of the public command surface.", "Search results are expected to be non-empty when you use them as a verification step."},
		SeeAlso:   []string{"ls", "read", "roots"},
		SortOrder: 50,
	},
	"create": {
		Key:       "create",
		Summary:   "Create a first-class Hub object.",
		Usage:     []string{"agenthub create project <name> [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"},
		Examples:  []string{"agenthub create project demo", "agenthub create project migration-notes"},
		Notes:     []string{"The category comes after the verb to match the root-directory mental model.", "The current public create surface is `project`."},
		SeeAlso:   []string{"project", "log", "read"},
		SortOrder: 60,
	},
	"log": {
		Key:       "log",
		Summary:   "Append a structured log entry to a project.",
		Usage:     []string{"agenthub log <path> --action ACTION --summary <text-or-file> [--tags a,b] [--literal] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"},
		Examples:  []string{"agenthub log project/demo --action note --summary ./summary.md", "agenthub log project/demo --action review --summary \"Regression check complete\" --tags release,qa"},
		Notes:     []string{"`log` currently targets `project/<name>` paths.", "The summary may be literal text, stdin, or a local file path.", "Read the project again afterward to verify the log entry is present and non-empty."},
		SeeAlso:   []string{"create", "read", "write"},
		SortOrder: 70,
	},
	"import": {
		Key:     "import",
		Summary: "Bring local files or platform exports into Agent Hub.",
		Usage: []string{
			"agenthub import platform <platform> [--mode agent|files|all] [--zip FILE]",
			"agenthub import skill <local-dir> [--name NAME]",
			"agenthub import profile <local-file> [--category preferences|relationships|principles]",
			"agenthub import memory <local-file-or-dir>",
			"agenthub import project <local-file-or-dir> [--name NAME]",
		},
		Examples: []string{
			"agenthub import platform codex",
			"agenthub import skill ./demo-skill",
			"agenthub import profile ./profile.json",
			"agenthub import memory ./notes/",
			"agenthub import project ./demo-project --name imported",
		},
		Notes:     []string{"Import categories come after the verb so the command shape matches `ls/read/write`.", "If you already initialized a local Git mirror with `agenthub git init`, successful imports will sync into that mirror automatically.", "Use `import platform ...` for Claude/Codex platform capture flows and `import skill/profile/memory/project ...` for direct local content."},
		SeeAlso:   []string{"write", "git", "platform"},
		SortOrder: 80,
	},
	"token": {
		Key:       "token",
		Summary:   "Create short-lived tokens for sync or prepared skills upload workflows.",
		Usage:     []string{"agenthub token create --kind sync --purpose PURPOSE [--access push|pull|both] [--ttl-minutes N]", "agenthub token create --kind skills-upload --purpose PURPOSE [--platform PLATFORM] [--ttl-minutes N]"},
		Examples:  []string{"agenthub token create --kind sync --purpose backup --access both", "agenthub token create --kind skills-upload --purpose skills --platform claude-web"},
		Notes:     []string{"`sync` replaces the old `create_sync_token` mental model.", "`skills-upload` replaces the old `prepare_skills_upload` mental model.", "Successful output includes non-empty `token`, `expires_at`, and workflow-specific helper fields."},
		SeeAlso:   []string{"import", "sync"},
		SortOrder: 90,
	},
	"stats": {
		Key:       "stats",
		Summary:   "Show a quick summary of current Hub contents.",
		Usage:     []string{"agenthub stats [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"},
		Examples:  []string{"agenthub stats", "agenthub stats --json"},
		Notes:     []string{"Use this to confirm the Hub is non-empty after imports or writes.", "The human-readable view reports file, memory, profile, project, and skill counts."},
		SeeAlso:   []string{"status", "ls"},
		SortOrder: 100,
	},
	"git": {
		Key:       "git",
		Summary:   "Mirror local Agent Hub data into a local Git repository and keep it refreshed.",
		Usage:     []string{"agenthub git init [--output DIR]", "agenthub git pull"},
		Examples:  []string{"agenthub git init --output ./agenthub-export/git-mirror", "agenthub git pull"},
		Notes:     []string{"`git init` exports all non-secret local Hub data, initializes the repo when needed, and registers it as the active mirror.", "If `--output` is omitted, Agent Hub uses `local.git_mirror_path` from `config.json`; if it is missing, Agent Hub writes the default `./agenthub-export/git-mirror` into `config.json` first.", "`git pull` refreshes the active mirror from the current local Hub state.", "Secrets are not exported; vault only contributes scope metadata.", "GitHub sync still requires running `git add / git commit / git remote add origin / git push` in that directory."},
		SeeAlso:   []string{"import", "write"},
		SortOrder: 110,
	},
	"platform": {
		Key:       "platform",
		Summary:   "Inspect installed platform adapters and their managed entrypoints.",
		Usage:     []string{"agenthub platform ls", "agenthub platform show <platform>"},
		Examples:  []string{"agenthub platform ls", "agenthub platform show codex", "agenthub platform show claude"},
		Notes:     []string{"Use `platform ls` to see which adapters are installed and connected.", "Use `platform show <platform>` to inspect config paths, entrypoints, supported domains, and embedded chat usage examples."},
		SeeAlso:   []string{"connect", "disconnect", "import"},
		SortOrder: 120,
	},
	"platform ls": {
		Key:       "platform ls",
		Summary:   "List discovered platform adapters and whether they are installed and connected.",
		Usage:     []string{"agenthub platform ls"},
		Examples:  []string{"agenthub platform ls"},
		Notes:     []string{"This is the public replacement for using root `ls` to inspect platforms.", "Output includes the adapter id, install state, connection state, and config path."},
		SeeAlso:   []string{"platform", "platform show"},
		SortOrder: 121,
	},
	"platform show": {
		Key:       "platform show",
		Summary:   "Show detailed status and routing hints for one platform adapter.",
		Usage:     []string{"agenthub platform show <platform>"},
		Examples:  []string{"agenthub platform show codex", "agenthub platform show claude"},
		Notes:     []string{"Use this before `connect` or `import platform` when you need to confirm the adapter shape.", "The `Chat usage` line is the authoritative embedded command syntax for that platform."},
		SeeAlso:   []string{"platform ls", "connect", "import"},
		SortOrder: 122,
	},
	"connect": {
		Key:       "connect",
		Summary:   "Install or refresh the Agent Hub managed entrypoint for a platform inside the current local environment.",
		Usage:     []string{"agenthub connect <platform>"},
		Examples:  []string{"agenthub connect codex", "agenthub connect claude"},
		Notes:     []string{"This command targets the current local environment; in isolated tests it should run under a temporary HOME/XDG root.", "A successful result reports the managed entrypoint path and embedded chat usage examples."},
		SeeAlso:   []string{"platform show", "disconnect"},
		SortOrder: 130,
	},
	"disconnect": {
		Key:       "disconnect",
		Summary:   "Remove an Agent Hub managed platform entrypoint and stored connection metadata.",
		Usage:     []string{"agenthub disconnect <platform>"},
		Examples:  []string{"agenthub disconnect codex", "agenthub disconnect claude"},
		Notes:     []string{"Use this when you want to remove the Agent Hub managed skill or command file from the current environment.", "This is operational cleanup, not a public Hub data command."},
		SeeAlso:   []string{"connect", "platform show"},
		SortOrder: 140,
	},
	"export": {
		Key:       "export",
		Summary:   "Stage platform-oriented export materials from the current local Hub state.",
		Usage:     []string{"agenthub export <platform> [--output DIR]"},
		Examples:  []string{"agenthub export codex --output ./codex-export", "agenthub export claude --output ./claude-export"},
		Notes:     []string{"Use this when you want platform-shaped export materials, not a Git mirror of the Hub itself.", "If the user wants a repo mirror of the Hub, prefer `agenthub git init` instead."},
		SeeAlso:   []string{"git", "platform"},
		SortOrder: 150,
	},
	"status": {
		Key:       "status",
		Summary:   "Show whether the local daemon and configured local storage are ready to use.",
		Usage:     []string{"agenthub status"},
		Examples:  []string{"agenthub status"},
		Notes:     []string{"This is the quickest operational readiness check.", "The output reports local daemon state, local storage backend, and current remote profile selection."},
		SeeAlso:   []string{"doctor", "stats"},
		SortOrder: 160,
	},
	"browse": {
		Key:       "browse",
		Summary:   "Open the local Agent Hub dashboard or print its authenticated URL.",
		Usage:     []string{"agenthub browse [--print-url] [/route]"},
		Examples:  []string{"agenthub browse", "agenthub browse --print-url /data/files"},
		Notes:     []string{"Use `--print-url` in scripts or terminal-only environments.", "The route is resolved relative to the local dashboard root."},
		SeeAlso:   []string{"status"},
		SortOrder: 170,
	},
	"doctor": {
		Key:       "doctor",
		Summary:   "Run a concise local readiness diagnostic.",
		Usage:     []string{"agenthub doctor"},
		Examples:  []string{"agenthub doctor"},
		Notes:     []string{"Use this when `status` is not enough and you want pointed next-step diagnostics."},
		SeeAlso:   []string{"status"},
		SortOrder: 180,
	},
	"daemon": {
		Key:       "daemon",
		Summary:   "Inspect or manage the local Agent Hub daemon process.",
		Usage:     []string{"agenthub daemon status", "agenthub daemon logs [--tail N]", "agenthub daemon stop"},
		Examples:  []string{"agenthub daemon status", "agenthub daemon logs --tail 50", "agenthub daemon stop"},
		Notes:     []string{"The public Hub data commands start the local daemon on demand when needed.", "Use this when you explicitly want daemon-level diagnostics or cleanup."},
		SeeAlso:   []string{"status", "doctor"},
		SortOrder: 190,
	},
	"sync": {
		Key:       "sync",
		Summary:   "Manage bundle-style sync workflows against a remote Agent Hub profile or archive transport.",
		Usage:     []string{"agenthub sync <subcommand>"},
		Examples:  []string{"agenthub sync profiles", "agenthub sync login --profile official", "agenthub sync push --bundle backup.ahub"},
		Notes:     []string{"`sync` is an operational workflow surface and is separate from the root-directory Hub commands.", "Use `agenthub token create --kind sync` when you need an ephemeral sync token."},
		SeeAlso:   []string{"token", "remote"},
		SortOrder: 200,
	},
	"remote": {
		Key:       "remote",
		Summary:   "Manage named remote profiles outside the bundle-oriented sync flow.",
		Usage:     []string{"agenthub remote <subcommand>"},
		Examples:  []string{"agenthub remote login local --url http://127.0.0.1:42690 --token ...", "agenthub remote use local", "agenthub remote whoami local"},
		Notes:     []string{"Use this when you need a named remote API base and token pairing."},
		SeeAlso:   []string{"sync"},
		SortOrder: 210,
	},
	"server": {
		Key:       "server",
		Summary:   "Start the standalone Agent Hub HTTP server.",
		Usage:     []string{"agenthub server [flags]"},
		Examples:  []string{"agenthub server --listen 127.0.0.1:42690 --local-mode"},
		Notes:     []string{"This is mostly for explicit server operation, not day-to-day local CLI use."},
		SeeAlso:   []string{"mcp"},
		SortOrder: 220,
	},
	"mcp": {
		Key:       "mcp",
		Summary:   "Run the Agent Hub MCP server over stdio.",
		Usage:     []string{"agenthub mcp stdio [flags]"},
		Examples:  []string{"agenthub mcp stdio --token-env AGENTHUB_TOKEN"},
		Notes:     []string{"This is the low-level MCP entrypoint used by managed platform integrations."},
		SeeAlso:   []string{"server", "connect"},
		SortOrder: 230,
	},
}

var cliHelpAliases = map[string]string{
	"root":         "",
	"overview":     "",
	"paths":        "roots",
	"path":         "roots",
	"profile":      "roots",
	"profiles":     "roots",
	"memory":       "roots",
	"memories":     "roots",
	"project":      "roots",
	"projects":     "roots",
	"skill":        "roots",
	"skills":       "roots",
	"secret":       "roots",
	"secrets":      "roots",
	"platforms":    "platform",
	"list":         "ls",
	"token create": "token",
	"git init":     "git",
	"git pull":     "git",
}

func runHelp(args []string) int {
	if len(args) == 0 {
		printRootUsage()
		return 0
	}
	if printHelpTopic(strings.Join(args, " ")) {
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown help topic %q\n\n", strings.Join(args, " "))
	fmt.Fprintf(os.Stderr, "available topics: %s\n\n", strings.Join(helpTopicsList(), ", "))
	printRootUsage()
	return 2
}

func printRootUsage() {
	fmt.Print(`Agent Hub

Root-directory command surface for local and remote Agent Hub data.

Mental model:
  - Start at the Hub root with agenthub ls
  - Public roots: profile, memory, project, skill, secret, platform
  - A leading / is optional. project/demo and /project/demo are equivalent.

Public commands:
  agenthub help [topic]                              Show root help or a topic-specific guide
  agenthub ls [path]                                 Browse public roots or a subtree
  agenthub read <path>                               Read a file, summary view, or secret
  agenthub write <path> <content-or-file>            Create or update Hub content
  agenthub search <query> [path]                     Search Hub content
  agenthub create <category> <name>                  Create a first-class Hub object
  agenthub log <path> --action ACTION --summary ...  Append a project log entry
  agenthub import <category> <src>                   Import local or platform data
  agenthub token create --kind sync|skills-upload    Create a short-lived workflow token
  agenthub stats                                     Show a quick Hub summary
  agenthub git init [--output DIR]                   Create the local Git mirror
  agenthub git pull                                  Refresh the active Git mirror

Operational commands:
  agenthub platform ls
  agenthub platform show <platform>
  agenthub connect <platform>
  agenthub disconnect <platform>
  agenthub export <platform> [--output DIR]
  agenthub browse [/route]
  agenthub status
  agenthub doctor
  agenthub daemon status|stop|logs
  agenthub sync <subcommand>
  agenthub remote <subcommand>
  agenthub server [flags]
  agenthub mcp stdio [flags]

Examples:
  agenthub ls
  agenthub read profile/preferences
  agenthub write memory "Remember this"
  agenthub create project demo
  agenthub import skill ./demo-skill
  agenthub git init --output ./agenthub-export/git-mirror

More help:
  agenthub help roots
  agenthub help write
  agenthub help import
  agenthub help git
`)
}

func printHelpTopic(raw string) bool {
	topic, ok := lookupHelpTopic(raw)
	if !ok {
		return false
	}
	fmt.Printf("%s\n\n", topicHeading(topic.Key))
	fmt.Printf("%s\n\n", topic.Summary)
	if len(topic.Usage) > 0 {
		fmt.Println("Usage:")
		for _, line := range topic.Usage {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
	}
	if len(topic.Examples) > 0 {
		fmt.Println("Examples:")
		for _, line := range topic.Examples {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
	}
	if len(topic.Notes) > 0 {
		fmt.Println("Notes:")
		for _, line := range topic.Notes {
			fmt.Printf("  - %s\n", line)
		}
		fmt.Println()
	}
	if len(topic.SeeAlso) > 0 {
		fmt.Printf("See also: %s\n", strings.Join(topic.SeeAlso, ", "))
	}
	return true
}

func topicHeading(key string) string {
	if key == "roots" {
		return "Agent Hub Path Model"
	}
	return fmt.Sprintf("agenthub %s", key)
}

func lookupHelpTopic(raw string) (cliHelpTopic, bool) {
	key := normalizeHelpTopic(raw)
	if alias, ok := cliHelpAliases[key]; ok {
		key = alias
	}
	if key == "" {
		return cliHelpTopic{}, false
	}
	topic, ok := cliHelpTopics[key]
	return topic, ok
}

func normalizeHelpTopic(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimPrefix(raw, "agenthub ")
	raw = strings.TrimPrefix(raw, "/")
	raw = strings.Join(strings.Fields(raw), " ")
	return raw
}

func isExplicitHelpRequest(args []string) bool {
	if isHelpArg(args) {
		return true
	}
	return containsFlag(args, "--help", "-h")
}

func helpTopicsList() []string {
	topics := make([]cliHelpTopic, 0, len(cliHelpTopics))
	for _, topic := range cliHelpTopics {
		if topic.Hidden {
			continue
		}
		topics = append(topics, topic)
	}
	sort.Slice(topics, func(i, j int) bool {
		if topics[i].SortOrder == topics[j].SortOrder {
			return topics[i].Key < topics[j].Key
		}
		return topics[i].SortOrder < topics[j].SortOrder
	})
	out := make([]string, 0, len(topics))
	for _, topic := range topics {
		out = append(out, topic.Key)
	}
	return out
}
