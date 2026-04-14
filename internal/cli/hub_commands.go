package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/api"
	"github.com/agi-bar/neudrive/internal/localgitsync"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
)

type hubCommandOptions struct {
	Profile string
	APIBase string
	Token   string
	Output  string
	JSON    bool
	Literal bool
}

type hubTarget struct {
	APIBase string
	Token   string
}

type hubSearchResponse struct {
	Query   string          `json:"query"`
	Results []api.SearchHit `json:"results"`
}

type hubProfileResponse struct {
	Slug        string                 `json:"slug"`
	DisplayName string                 `json:"display_name"`
	Timezone    string                 `json:"timezone"`
	Language    string                 `json:"language"`
	Profiles    []models.MemoryProfile `json:"profiles"`
}

type hubProjectsResponse struct {
	Projects []models.Project `json:"projects"`
}

type hubProjectResponse struct {
	Project models.Project      `json:"project"`
	Logs    []models.ProjectLog `json:"logs"`
}

type hubSkillsResponse struct {
	Skills []models.SkillSummary `json:"skills"`
}

type hubVaultScopesResponse struct {
	Scopes []models.VaultScope `json:"scopes"`
}

type hubVaultReadResponse struct {
	Scope string `json:"scope"`
	Data  string `json:"data"`
}

type hubTokenCreateResponse struct {
	Token                   string   `json:"token"`
	ExpiresAt               string   `json:"expires_at"`
	APIBase                 string   `json:"api_base"`
	Scopes                  []string `json:"scopes"`
	Usage                   string   `json:"usage"`
	UploadURL               string   `json:"upload_url,omitempty"`
	BrowserUploadURL        string   `json:"browser_upload_url,omitempty"`
	ConnectivityProbeURL    string   `json:"connectivity_probe_url,omitempty"`
	ConnectivityProbeMethod string   `json:"connectivity_probe_method,omitempty"`
	CurlExample             string   `json:"curl_example,omitempty"`
	Warning                 string   `json:"warning,omitempty"`
}

type hubTreeImportRequest struct {
	Files map[string]string `json:"files"`
}

func runHubLS(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("ls")
		return 0
	}
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, usageLine("ls [path] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}
	targetPath := ""
	if fs.NArg() == 1 {
		targetPath = fs.Arg(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	node, err := hubListNode(ctx, target, targetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ls: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, renderListText(node), node); err != nil {
		fmt.Fprintf(os.Stderr, "ls: %v\n", err)
		return 1
	}
	return 0
}

func runHubRead(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("read")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 1)
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("read <path> [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	text, payload, err := hubRead(ctx, target, positionals[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, text, payload); err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		return 1
	}
	return 0
}

func runHubWrite(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("write")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 2)
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := bindHubCommandFlags(fs, true)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 2 {
		fmt.Fprintln(os.Stderr, usageLine("write <path> <content-or-file> [--literal] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}

	content, err := readCLIContentArg(positionals[1], opts.Literal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	text, payload, syncInfo, err := hubWrite(ctx, target, positionals[0], content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, text, payload); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		return 1
	}
	printLocalGitSyncMessage(syncInfo)
	return 0
}

func runHubSearch(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("search")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 2)
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) < 1 || len(positionals) > 2 {
		fmt.Fprintln(os.Stderr, usageLine("search <query> [path] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	scope := ""
	if len(positionals) == 2 {
		scope, err = externalPathToSearchScope(positionals[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "search: %v\n", err)
			return 1
		}
	}
	resp, err := hubSearch(ctx, target, positionals[0], scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, renderSearchText(resp), resp); err != nil {
		fmt.Fprintf(os.Stderr, "search: %v\n", err)
		return 1
	}
	return 0
}

func runHubCreate(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("create")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 2)
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 2 {
		fmt.Fprintln(os.Stderr, usageLine("create <category> <name> [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}
	if normalizeExternalCategory(positionals[0]) != "project" {
		fmt.Fprintln(os.Stderr, "create currently supports only project")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	payload := map[string]string{"name": positionals[1]}
	var project models.Project
	syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, "/agent/projects", payload, &project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, fmt.Sprintf("Created project/%s.", project.Name), project); err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		return 1
	}
	printLocalGitSyncMessage(syncInfo)
	return 0
}

func runHubLog(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("log")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 1)
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	action := fs.String("action", "", "log action name")
	summary := fs.String("summary", "", "summary text or local file path")
	tags := fs.String("tags", "", "comma-separated tags")
	opts := bindHubCommandFlags(fs, true)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("log <path> --action ACTION --summary <text-or-file> [--tags a,b] [--literal] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}
	if strings.TrimSpace(*action) == "" || strings.TrimSpace(*summary) == "" {
		fmt.Fprintln(os.Stderr, usageLine("log <path> --action ACTION --summary <text-or-file> [--tags a,b]"))
		return 2
	}

	resolved, err := parseExternalPath(positionals[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: %v\n", err)
		return 1
	}
	if resolved.Category != "project" || strings.TrimSpace(resolved.Name) == "" {
		fmt.Fprintln(os.Stderr, "log currently supports project/<name>")
		return 2
	}
	content, err := readCLIContentArg(*summary, opts.Literal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	req := map[string]any{
		"source":  "neudrive-cli",
		"action":  *action,
		"summary": content,
	}
	if tagList := parseCommaSeparated(*tags); len(tagList) > 0 {
		req["tags"] = tagList
	}
	var resp map[string]string
	apiPath := "/agent/projects/" + url.PathEscape(resolved.Name) + "/log"
	syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, apiPath, req, &resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, fmt.Sprintf("Logged %s on project/%s.", *action, resolved.Name), resp); err != nil {
		fmt.Fprintf(os.Stderr, "log: %v\n", err)
		return 1
	}
	printLocalGitSyncMessage(syncInfo)
	return 0
}

func runHubImport(args []string) int {
	if len(args) == 0 || isExplicitHelpRequest(args) {
		printHelpTopic("import")
		return 0
	}
	switch normalizeExternalCategory(args[0]) {
	case "platform":
		return runHubImportPlatform(args[1:])
	case "skill":
		return runHubImportSkill(args[1:])
	case "profile":
		return runHubImportProfile(args[1:])
	case "memory":
		return runHubImportMemory(args[1:])
	case "project":
		return runHubImportProject(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown import category %q\n", args[0])
		return 2
	}
}

func runHubImportPlatform(args []string) int {
	if len(args) == 0 || isExplicitHelpRequest(args) {
		printHelpTopic("import")
		return 0
	}
	return runLegacyImport(args)
}

func runHubImportSkill(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("import")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 1)
	fs := flag.NewFlagSet("import skill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "skill name override")
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("import skill <local-dir> [--name NAME] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}
	srcDir, err := resolveExistingLocalDir(positionals[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "import skill: %v\n", err)
		return 1
	}
	skillName := strings.TrimSpace(*name)
	if skillName == "" {
		skillName = filepath.Base(srcDir)
	}
	files, err := loadTextTree(srcDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import skill: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	var resp api.ImportResponse
	req := map[string]any{"name": skillName, "files": files}
	syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, "/agent/import/skill", req, &resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import skill: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, fmt.Sprintf("Imported skill/%s from %s.", skillName, srcDir), resp); err != nil {
		fmt.Fprintf(os.Stderr, "import skill: %v\n", err)
		return 1
	}
	printLocalGitSyncMessage(syncInfo)
	return 0
}

func runHubImportProfile(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("import")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 1)
	fs := flag.NewFlagSet("import profile", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	category := fs.String("category", "", "profile category: preferences, relationships, or principles")
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("import profile <local-file> [--category preferences|relationships|principles] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}
	content, err := readCLIContentArg(positionals[0], false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import profile: %v\n", err)
		return 1
	}
	profileFields, err := buildProfileImportPayload(content, strings.TrimSpace(*category))
	if err != nil {
		fmt.Fprintf(os.Stderr, "import profile: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	var resp api.ImportResponse
	syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, "/agent/import/profile", profileFields, &resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import profile: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, fmt.Sprintf("Imported %d profile categories.", resp.Data.ImportedCount), resp); err != nil {
		fmt.Fprintf(os.Stderr, "import profile: %v\n", err)
		return 1
	}
	printLocalGitSyncMessage(syncInfo)
	return 0
}

func runHubImportMemory(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("import")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 1)
	fs := flag.NewFlagSet("import memory", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("import memory <local-file-or-dir> [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}
	src := positionals[0]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	info, err := os.Stat(expandCLIUser(src))
	if err != nil {
		if src == "-" {
			info = nil
		} else {
			fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
			return 1
		}
	}

	if info != nil && info.IsDir() {
		root, err := resolveExistingLocalDir(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
			return 1
		}
		files, err := loadTextTree(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
			return 1
		}
		importedRoot := path.Join("/memory/imported", filepath.Base(root))
		resp, syncInfo, err := importBulkTextTree(ctx, target, importedRoot, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
			return 1
		}
		if err := writeHubOutput(opts, fmt.Sprintf("Imported %d files into memory/%s.", resp.Data.ImportedCount, path.Join("imported", filepath.Base(root))), resp); err != nil {
			fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
			return 1
		}
		printLocalGitSyncMessage(syncInfo)
		return 0
	}

	content, err := readCLIContentArg(src, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
		return 1
	}

	resp, syncInfo, err := importMemoryContent(ctx, target, src, content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, renderImportText("memory", resp), resp); err != nil {
		fmt.Fprintf(os.Stderr, "import memory: %v\n", err)
		return 1
	}
	printLocalGitSyncMessage(syncInfo)
	return 0
}

func runHubImportProject(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("import")
		return 0
	}
	leading, flagArgs := splitLeadingPositionals(args, 1)
	fs := flag.NewFlagSet("import project", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "project name override")
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	positionals := append(leading, fs.Args()...)
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("import project <local-file-or-dir> [--name NAME] [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}
	src := positionals[0]
	absSrc := expandCLIUser(src)
	info, err := os.Stat(absSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import project: %v\n", err)
		return 1
	}
	projectName := strings.TrimSpace(*name)
	if projectName == "" {
		base := filepath.Base(absSrc)
		projectName = strings.TrimSuffix(base, filepath.Ext(base))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}
	if err := ensureProjectExists(ctx, target, projectName); err != nil {
		fmt.Fprintf(os.Stderr, "import project: %v\n", err)
		return 1
	}

	if info.IsDir() {
		root, err := resolveExistingLocalDir(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import project: %v\n", err)
			return 1
		}
		files, err := loadTextTree(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import project: %v\n", err)
			return 1
		}
		projectRoot := path.Join("/projects", projectName)
		resp, gitInfo, err := importBulkTextTree(ctx, target, projectRoot, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import project: %v\n", err)
			return 1
		}
		if err := writeHubOutput(opts, fmt.Sprintf("Imported %d files into project/%s.", resp.Data.ImportedCount, projectName), resp); err != nil {
			fmt.Fprintf(os.Stderr, "import project: %v\n", err)
			return 1
		}
		printLocalGitSyncMessage(gitInfo)
		return 0
	}

	content, err := readCLIContentArg(src, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import project: %v\n", err)
		return 1
	}
	relPath := filepath.Base(absSrc)
	req := api.ImportBulkRequest{
		Files: map[string]string{
			path.Join("/projects", projectName, relPath): content,
		},
	}
	var importResp api.ImportResponse
	gitInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, "/agent/import/bulk", req, &importResp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import project: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, fmt.Sprintf("Imported %s into project/%s.", relPath, projectName), importResp); err != nil {
		fmt.Fprintf(os.Stderr, "import project: %v\n", err)
		return 1
	}
	printLocalGitSyncMessage(gitInfo)
	return 0
}

func runHubToken(args []string) int {
	if len(args) == 0 || isExplicitHelpRequest(args) {
		printHelpTopic("token")
		return 0
	}
	switch args[0] {
	case "create":
		return runHubTokenCreate(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown token subcommand %q\n", args[0])
		return 2
	}
}

func runHubTokenCreate(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("token")
		return 0
	}
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	kind := fs.String("kind", "", "token kind: sync or skills-upload")
	purpose := fs.String("purpose", "", "token purpose label")
	access := fs.String("access", "push", "sync token access: push, pull, or both")
	platform := fs.String("platform", "claude-web", "skills upload platform hint")
	ttlMinutes := fs.Int("ttl-minutes", 30, "ephemeral token TTL in minutes")
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*kind) == "" || strings.TrimSpace(*purpose) == "" {
		fmt.Fprintln(os.Stderr, usageLine("token create --kind sync|skills-upload --purpose PURPOSE [--access push|pull|both] [--platform PLATFORM] [--ttl-minutes N]"))
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	req := map[string]any{
		"kind":        *kind,
		"purpose":     *purpose,
		"access":      *access,
		"platform":    *platform,
		"ttl_minutes": *ttlMinutes,
	}
	var resp hubTokenCreateResponse
	if err := localAPIPostJSON(ctx, target.APIBase, target.Token, "/agent/tokens/ephemeral", req, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "token create: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, renderTokenText(resp), resp); err != nil {
		fmt.Fprintf(os.Stderr, "token create: %v\n", err)
		return 1
	}
	return 0
}

func runHubStats(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("stats")
		return 0
	}
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := bindHubCommandFlags(fs, false)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, usageLine("stats [--json] [--output FILE] [--profile NAME | --api-base URL --token TOKEN]"))
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveHubTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare hub target: %v\n", err)
		return 1
	}

	var payload map[string]any
	if err := localAPIGet(ctx, target.APIBase, target.Token, "/agent/dashboard/stats", &payload); err != nil {
		fmt.Fprintf(os.Stderr, "stats: %v\n", err)
		return 1
	}
	if err := writeHubOutput(opts, renderKeyValueText(payload), payload); err != nil {
		fmt.Fprintf(os.Stderr, "stats: %v\n", err)
		return 1
	}
	return 0
}

func bindHubCommandFlags(fs *flag.FlagSet, includeLiteral bool) *hubCommandOptions {
	opts := &hubCommandOptions{}
	fs.StringVar(&opts.Profile, "profile", "", "explicit remote profile name")
	fs.StringVar(&opts.APIBase, "api-base", "", "explicit neuDrive API base")
	fs.StringVar(&opts.Token, "token", "", "explicit scoped token")
	fs.BoolVar(&opts.JSON, "json", false, "output JSON")
	fs.StringVar(&opts.Output, "output", "", "write final output to a local file")
	if includeLiteral {
		fs.BoolVar(&opts.Literal, "literal", false, "treat text arguments literally instead of reading local files")
	}
	return opts
}

func resolveHubTarget(ctx context.Context, opts *hubCommandOptions) (*hubTarget, error) {
	if opts == nil {
		opts = &hubCommandOptions{}
	}
	if strings.TrimSpace(opts.Profile) != "" {
		_, cfg, err := runtimecfg.LoadConfig("")
		if err != nil {
			return nil, err
		}
		profile, ok := cfg.Profiles[strings.TrimSpace(opts.Profile)]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", opts.Profile)
		}
		if strings.TrimSpace(profile.APIBase) == "" || strings.TrimSpace(profile.Token) == "" {
			return nil, fmt.Errorf("profile %q is missing api_base or token", opts.Profile)
		}
		return &hubTarget{APIBase: strings.TrimSpace(profile.APIBase), Token: strings.TrimSpace(profile.Token)}, nil
	}
	if strings.TrimSpace(opts.APIBase) != "" || strings.TrimSpace(opts.Token) != "" {
		if strings.TrimSpace(opts.APIBase) == "" || strings.TrimSpace(opts.Token) == "" {
			return nil, errors.New("--api-base and --token must be provided together")
		}
		return &hubTarget{APIBase: strings.TrimSpace(opts.APIBase), Token: strings.TrimSpace(opts.Token)}, nil
	}
	_, state, token, err := ensureLocalOwnerAccessForAPI(ctx)
	if err != nil {
		return nil, err
	}
	return &hubTarget{APIBase: state.APIBase, Token: token}, nil
}

func hubListNode(ctx context.Context, target *hubTarget, rawPath string) (*api.FileNode, error) {
	resolved, err := parseExternalPath(rawPath)
	if err != nil {
		return nil, err
	}
	switch resolved.Category {
	case "":
		return syntheticRootNode(), nil
	case "profile":
		return hubListProfile(ctx, target)
	case "memory":
		if resolved.Rest == "" {
			return hubListMemory(ctx, target)
		}
		return hubListTree(ctx, target, resolved.InternalPath)
	case "project":
		if resolved.Name == "" {
			return hubListProjects(ctx, target)
		}
		return hubListTree(ctx, target, resolved.InternalPath)
	case "skill":
		if resolved.Name == "" {
			return hubListSkills(ctx, target)
		}
		return hubListTree(ctx, target, resolved.InternalPath)
	case "secret":
		if resolved.Rest != "" {
			return nil, errors.New("ls secret only supports the secret root")
		}
		return hubListSecrets(ctx, target)
	case "platform":
		if resolved.Name == "" && resolved.Rest == "" {
			return hubListTree(ctx, target, "/platforms/")
		}
		return hubListTree(ctx, target, resolved.InternalPath)
	default:
		return nil, fmt.Errorf("unsupported path %q", rawPath)
	}
}

func hubRead(ctx context.Context, target *hubTarget, rawPath string) (string, any, error) {
	resolved, err := parseExternalPath(rawPath)
	if err != nil {
		return "", nil, err
	}
	switch resolved.Category {
	case "":
		node := syntheticRootNode()
		return renderListText(node), node, nil
	case "profile":
		if resolved.Name == "" {
			node, err := hubListProfile(ctx, target)
			if err != nil {
				return "", nil, err
			}
			return renderListText(node), node, nil
		}
	case "project":
		if resolved.Name != "" && resolved.Rest == resolved.Name {
			var resp hubProjectResponse
			apiPath := "/agent/projects/" + url.PathEscape(resolved.Name)
			if err := localAPIGet(ctx, target.APIBase, target.Token, apiPath, &resp); err != nil {
				return "", nil, err
			}
			return renderProjectText(resp), resp, nil
		}
	case "secret":
		if strings.TrimSpace(resolved.Name) == "" {
			return "", nil, errors.New("read secret expects secret/<scope>")
		}
		var resp hubVaultReadResponse
		apiPath := "/agent/vault/" + url.PathEscape(resolved.Name)
		if err := localAPIGet(ctx, target.APIBase, target.Token, apiPath, &resp); err != nil {
			return "", nil, err
		}
		text := resp.Data
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text, resp, nil
	}

	var node api.FileNode
	if err := localAPIGet(ctx, target.APIBase, target.Token, "/agent/tree"+resolved.InternalPath, &node); err != nil {
		return "", nil, err
	}
	externalizeNode(&node)
	if node.IsDir {
		return renderListText(&node), &node, nil
	}
	if node.Content == "" && !isTextLikeContent(node.MimeType) {
		return "", nil, fmt.Errorf("%s is a binary file (%s)", node.Path, node.MimeType)
	}
	text := node.Content
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, &node, nil
}

func hubWrite(ctx context.Context, target *hubTarget, rawPath, content string) (string, any, *localgitsync.SyncInfo, error) {
	resolved, err := parseExternalPath(rawPath)
	if err != nil {
		return "", nil, nil, err
	}
	switch resolved.Category {
	case "profile":
		if strings.TrimSpace(resolved.Name) == "" {
			return "", nil, nil, errors.New("write profile expects profile/<category>")
		}
		req := map[string]any{
			"category": resolved.Name,
			"content":  content,
			"source":   "neudrive-cli",
		}
		var profile hubProfileResponse
		syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPut, target.APIBase, target.Token, "/agent/memory/profile", req, &profile)
		return fmt.Sprintf("Updated profile/%s.", resolved.Name), profile, syncInfo, err
	case "memory":
		if strings.TrimSpace(resolved.Rest) == "" {
			req := map[string]any{"content": content, "source": "neudrive-cli"}
			var resp api.ImportResponse
			syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, "/agent/memory/scratch", req, &resp)
			return "Saved memory note.", resp, syncInfo, err
		}
	case "secret":
		return "", nil, nil, errors.New("write secret is not supported")
	}
	if strings.HasSuffix(resolved.InternalPath, "/") || resolved.InternalPath == "/" {
		return "", nil, nil, errors.New("write expects a file path, not a directory")
	}
	contentType := guessContentType(resolved.InternalPath)
	req := map[string]any{
		"content":      content,
		"content_type": contentType,
	}
	var node api.FileNode
	syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPut, target.APIBase, target.Token, "/agent/tree"+resolved.InternalPath, req, &node)
	if err != nil {
		return "", nil, nil, err
	}
	externalizeNode(&node)
	return fmt.Sprintf("Wrote %s.", node.Path), &node, syncInfo, nil
}

func hubSearch(ctx context.Context, target *hubTarget, query, scope string) (*hubSearchResponse, error) {
	apiPath := "/agent/search?q=" + url.QueryEscape(query)
	if strings.TrimSpace(scope) != "" {
		apiPath += "&scope=" + url.QueryEscape(scope)
	}
	var resp hubSearchResponse
	if err := localAPIGet(ctx, target.APIBase, target.Token, apiPath, &resp); err != nil {
		return nil, err
	}
	filtered := make([]api.SearchHit, 0, len(resp.Results))
	for _, hit := range resp.Results {
		publicPath, ok := externalizeHubPath(hit.Path)
		if !ok {
			continue
		}
		hit.Path = publicPath
		filtered = append(filtered, hit)
	}
	resp.Results = filtered
	return &resp, nil
}

func hubListProfile(ctx context.Context, target *hubTarget) (*api.FileNode, error) {
	node, err := hubListTree(ctx, target, "/memory/profile/")
	if err != nil {
		return nil, err
	}
	node.Path = "profile/"
	node.Name = "profile"
	return node, nil
}

func hubListMemory(ctx context.Context, target *hubTarget) (*api.FileNode, error) {
	node, err := hubListTree(ctx, target, "/memory/")
	if err != nil {
		return nil, err
	}
	filtered := make([]*api.FileNode, 0, len(node.Children))
	for _, child := range node.Children {
		if child.Path == "memory/profile/" {
			continue
		}
		filtered = append(filtered, child)
	}
	node.Children = filtered
	node.Path = "memory/"
	node.Name = "memory"
	return node, nil
}

func hubListProjects(ctx context.Context, target *hubTarget) (*api.FileNode, error) {
	var resp hubProjectsResponse
	if err := localAPIGet(ctx, target.APIBase, target.Token, "/agent/projects", &resp); err != nil {
		return nil, err
	}
	children := make([]*api.FileNode, 0, len(resp.Projects))
	for _, project := range resp.Projects {
		children = append(children, &api.FileNode{
			Path:  "project/" + project.Name + "/",
			Name:  project.Name,
			IsDir: true,
			Kind:  "directory",
		})
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return &api.FileNode{Path: "project/", Name: "project", IsDir: true, Children: children}, nil
}

func hubListSkills(ctx context.Context, target *hubTarget) (*api.FileNode, error) {
	var resp hubSkillsResponse
	if err := localAPIGet(ctx, target.APIBase, target.Token, "/agent/skills", &resp); err != nil {
		return nil, err
	}
	children := make([]*api.FileNode, 0, len(resp.Skills))
	for _, skill := range resp.Skills {
		children = append(children, &api.FileNode{
			Path:  "skill/" + skill.Name + "/",
			Name:  skill.Name,
			IsDir: true,
			Kind:  "directory",
		})
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return &api.FileNode{Path: "skill/", Name: "skill", IsDir: true, Children: children}, nil
}

func hubListSecrets(ctx context.Context, target *hubTarget) (*api.FileNode, error) {
	var resp hubVaultScopesResponse
	if err := localAPIGet(ctx, target.APIBase, target.Token, "/agent/vault/scopes", &resp); err != nil {
		return nil, err
	}
	children := make([]*api.FileNode, 0, len(resp.Scopes))
	for _, scope := range resp.Scopes {
		children = append(children, &api.FileNode{
			Path:  "secret/" + scope.Scope,
			Name:  scope.Scope,
			IsDir: false,
			Kind:  "secret_scope",
		})
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return &api.FileNode{Path: "secret/", Name: "secret", IsDir: true, Children: children}, nil
}

func hubListTree(ctx context.Context, target *hubTarget, internalPath string) (*api.FileNode, error) {
	var node api.FileNode
	if err := localAPIGet(ctx, target.APIBase, target.Token, "/agent/tree"+internalPath, &node); err != nil {
		return nil, err
	}
	externalizeNode(&node)
	return &node, nil
}

type externalPath struct {
	Category     string
	Name         string
	Rest         string
	InternalPath string
}

func parseExternalPath(raw string) (*externalPath, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return &externalPath{}, nil
	}
	trimmed = strings.TrimPrefix(trimmed, "./")
	parts := strings.Split(trimmed, "/")
	category := normalizeExternalCategory(parts[0])
	if category == "" {
		return nil, fmt.Errorf("unsupported root %q; expected profile, memory, project, skill, secret, or platform", parts[0])
	}
	restParts := parts[1:]
	rest := strings.Join(restParts, "/")
	internalRest := rest
	switch category {
	case "profile":
		name := strings.TrimSuffix(rest, ".md")
		if name == "" {
			return &externalPath{Category: category, Rest: ""}, nil
		}
		if strings.Contains(name, "/") {
			return nil, errors.New("profile paths only support profile/<category>")
		}
		return &externalPath{
			Category:     category,
			Name:         name,
			Rest:         name,
			InternalPath: "/memory/profile/" + name + ".md",
		}, nil
	case "memory":
		if rest == "" {
			return &externalPath{Category: category, InternalPath: "/memory/"}, nil
		}
		return &externalPath{Category: category, Rest: rest, InternalPath: "/memory/" + internalRest}, nil
	case "project":
		if rest == "" {
			return &externalPath{Category: category, InternalPath: "/projects/"}, nil
		}
		name := restParts[0]
		return &externalPath{Category: category, Name: name, Rest: rest, InternalPath: "/projects/" + internalRest}, nil
	case "skill":
		if rest == "" {
			return &externalPath{Category: category, InternalPath: "/skills/"}, nil
		}
		name := restParts[0]
		return &externalPath{Category: category, Name: name, Rest: rest, InternalPath: "/skills/" + internalRest}, nil
	case "secret":
		if rest == "" {
			return &externalPath{Category: category}, nil
		}
		return &externalPath{Category: category, Name: rest, Rest: rest, InternalPath: "/vault/" + rest}, nil
	case "platform":
		if rest == "" {
			return &externalPath{Category: category, InternalPath: "/platforms/"}, nil
		}
		name := restParts[0]
		return &externalPath{Category: category, Name: name, Rest: rest, InternalPath: "/platforms/" + internalRest}, nil
	default:
		return nil, fmt.Errorf("unsupported path %q", raw)
	}
}

func normalizeExternalCategory(raw string) string {
	value := strings.Trim(strings.TrimSpace(raw), "/")
	switch strings.ToLower(value) {
	case "profile", "profiles":
		return "profile"
	case "memory", "memories":
		return "memory"
	case "project", "projects":
		return "project"
	case "skill", "skills":
		return "skill"
	case "secret", "secrets":
		return "secret"
	case "platform", "platforms":
		return "platform"
	default:
		return ""
	}
}

func syntheticRootNode() *api.FileNode {
	children := []*api.FileNode{
		{Path: "memory/", Name: "memory", IsDir: true},
		{Path: "platform/", Name: "platform", IsDir: true},
		{Path: "profile/", Name: "profile", IsDir: true},
		{Path: "project/", Name: "project", IsDir: true},
		{Path: "secret/", Name: "secret", IsDir: true},
		{Path: "skill/", Name: "skill", IsDir: true},
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return &api.FileNode{Path: "/", Name: "/", IsDir: true, Children: children}
}

func externalizeNode(node *api.FileNode) {
	if node == nil {
		return
	}
	if publicPath, ok := externalizeHubPath(node.Path); ok {
		node.Path = publicPath
	}
	node.Name = externalNodeName(node.Path)
	filtered := make([]*api.FileNode, 0, len(node.Children))
	for _, child := range node.Children {
		if child == nil {
			continue
		}
		if _, ok := externalizeHubPath(child.Path); !ok {
			continue
		}
		externalizeNode(child)
		filtered = append(filtered, child)
	}
	node.Children = filtered
}

func externalizeHubPath(raw string) (string, bool) {
	publicPath := normalizeHubPath(raw)
	switch {
	case publicPath == "/":
		return "/", true
	case strings.HasPrefix(publicPath, "/memory/profile/"):
		rel := strings.TrimPrefix(publicPath, "/memory/profile/")
		rel = strings.TrimSuffix(rel, ".md")
		if rel == "" {
			return "profile/", true
		}
		return "profile/" + rel, true
	case strings.HasPrefix(publicPath, "/memory/"):
		return strings.TrimPrefix(publicPath, "/"), true
	case strings.HasPrefix(publicPath, "/projects/"):
		return "project/" + strings.TrimPrefix(publicPath, "/projects/"), true
	case strings.HasPrefix(publicPath, "/skills/"):
		return "skill/" + strings.TrimPrefix(publicPath, "/skills/"), true
	case strings.HasPrefix(publicPath, "/vault/"):
		return "secret/" + strings.TrimPrefix(publicPath, "/vault/"), true
	case strings.HasPrefix(publicPath, "/platforms/"):
		return "platform/" + strings.TrimPrefix(publicPath, "/platforms/"), true
	default:
		return "", false
	}
}

func externalNodeName(pathValue string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(pathValue), "/")
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

func externalPathToSearchScope(raw string) (string, error) {
	resolved, err := parseExternalPath(raw)
	if err != nil {
		return "", err
	}
	switch resolved.Category {
	case "":
		return "all", nil
	case "profile":
		return "/memory/profile", nil
	case "memory":
		if resolved.Rest == "" {
			return "/memory", nil
		}
		return resolved.InternalPath, nil
	case "project", "skill", "platform":
		return resolved.InternalPath, nil
	case "secret":
		return "", errors.New("search secret is not supported")
	default:
		return "", fmt.Errorf("unsupported search scope %q", raw)
	}
}

func readCLIContentArg(raw string, literal bool) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if literal {
		return raw, nil
	}
	pathValue := expandCLIUser(value)
	info, err := os.Stat(pathValue)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory", value)
		}
		data, err := os.ReadFile(pathValue)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return raw, nil
}

func resolveExistingLocalDir(raw string) (string, error) {
	target, err := resolveCLIPath(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", raw)
	}
	return target, nil
}

func loadTextTree(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(current string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if looksBinary(data) {
			return fmt.Errorf("%s looks binary; use neudrive token create --kind skills-upload for archive-heavy imports", current)
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no files found")
	}
	return files, nil
}

func looksBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	limit := len(data)
	if limit > 8000 {
		limit = 8000
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}

func buildProfileImportPayload(content, category string) (map[string]string, error) {
	if strings.TrimSpace(category) != "" {
		switch category {
		case "preferences", "relationships", "principles":
			return map[string]string{category: content}, nil
		default:
			return nil, fmt.Errorf("unsupported profile category %q", category)
		}
	}
	var object map[string]string
	if err := json.Unmarshal([]byte(content), &object); err != nil {
		return nil, errors.New("profile import without --category expects a JSON object with preferences/relationships/principles")
	}
	payload := map[string]string{}
	for _, key := range []string{"preferences", "relationships", "principles"} {
		if strings.TrimSpace(object[key]) != "" {
			payload[key] = object[key]
		}
	}
	if len(payload) == 0 {
		return nil, errors.New("profile import JSON must include at least one of preferences, relationships, or principles")
	}
	return payload, nil
}

func importMemoryContent(ctx context.Context, target *hubTarget, src, content string) (api.ImportResponse, *localgitsync.SyncInfo, error) {
	var resp api.ImportResponse
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var envelope struct {
			Memories []map[string]any `json:"memories"`
		}
		if err := json.Unmarshal([]byte(content), &envelope); err == nil && len(envelope.Memories) > 0 {
			files := map[string]string{}
			for index, memory := range envelope.Memories {
				itemContent, _ := memory["content"].(string)
				if strings.TrimSpace(itemContent) == "" {
					continue
				}
				title, _ := memory["title"].(string)
				fileName := fmt.Sprintf("item-%02d.md", index+1)
				if strings.TrimSpace(title) != "" {
					fileName = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(title), " ", "-")) + ".md"
				}
				files[path.Join("/memory/imported/json", fileName)] = itemContent
			}
			if len(files) > 0 {
				out, syncInfo, err := importBulkTextTree(ctx, target, "/", files)
				return out, syncInfo, err
			}
		}
	}

	title := ""
	if src != "-" {
		title = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	}
	req := map[string]any{
		"content": content,
		"source":  "neudrive-cli",
		"title":   title,
	}
	syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, "/agent/memory/scratch", req, &resp)
	return resp, syncInfo, err
}

func importBulkTextTree(ctx context.Context, target *hubTarget, root string, files map[string]string) (api.ImportResponse, *localgitsync.SyncInfo, error) {
	payload := api.ImportBulkRequest{Files: map[string]string{}}
	for relPath, content := range files {
		if strings.HasPrefix(relPath, "/") {
			payload.Files[relPath] = content
			continue
		}
		payload.Files[path.Join(root, filepath.ToSlash(relPath))] = content
	}
	var resp api.ImportResponse
	syncInfo, err := localAPIJSONWithSync(ctx, http.MethodPost, target.APIBase, target.Token, "/agent/import/bulk", payload, &resp)
	return resp, syncInfo, err
}

func ensureProjectExists(ctx context.Context, target *hubTarget, name string) error {
	var existing hubProjectResponse
	err := localAPIGet(ctx, target.APIBase, target.Token, "/agent/projects/"+url.PathEscape(name), &existing)
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return err
	}
	var project models.Project
	return localAPIPostJSON(ctx, target.APIBase, target.Token, "/agent/projects", map[string]string{"name": name}, &project)
}

func guessContentType(internalPath string) string {
	ext := strings.ToLower(filepath.Ext(internalPath))
	if ext == ".md" {
		return "text/markdown"
	}
	if ext == ".jsonl" {
		return "application/x-ndjson"
	}
	if ct := mime.TypeByExtension(ext); strings.TrimSpace(ct) != "" {
		return ct
	}
	return "text/plain"
}

func parseCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func splitLeadingPositionals(args []string, max int) ([]string, []string) {
	if max <= 0 {
		return nil, args
	}
	leading := make([]string, 0, max)
	index := 0
	for index < len(args) && len(leading) < max {
		if strings.HasPrefix(args[index], "-") {
			break
		}
		leading = append(leading, args[index])
		index++
	}
	return leading, args[index:]
}

func resolveCLIPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("path is required")
	}
	return filepath.Abs(expandCLIUser(strings.TrimSpace(raw)))
}

func expandCLIUser(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(raw, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(raw, "~/"))
	}
	return raw
}

func writeHubOutput(opts *hubCommandOptions, text string, payload any) error {
	var data []byte
	var err error
	if opts != nil && opts.JSON {
		if payload == nil {
			payload = map[string]string{"message": strings.TrimSpace(text)}
		}
		data, err = json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	} else {
		if strings.TrimSpace(text) == "" && payload != nil {
			data, err = json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			data = append(data, '\n')
		} else {
			data = []byte(text)
			if len(data) > 0 && data[len(data)-1] != '\n' {
				data = append(data, '\n')
			}
		}
	}
	if opts != nil && strings.TrimSpace(opts.Output) != "" {
		target, err := resolveCLIPath(opts.Output)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}
	fmt.Print(string(data))
	return nil
}

func renderListText(node *api.FileNode) string {
	if node == nil {
		return ""
	}
	entries := node.Children
	if !node.IsDir {
		entries = []*api.FileNode{node}
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		kind := "file"
		if entry.IsDir {
			kind = "dir"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s", kind, entry.Path))
	}
	return strings.Join(lines, "\n")
}

func renderSearchText(resp *hubSearchResponse) string {
	if resp == nil || len(resp.Results) == 0 {
		return "No matches found."
	}
	lines := make([]string, 0, len(resp.Results))
	for _, hit := range resp.Results {
		line := hit.Path
		if snippet := strings.TrimSpace(hit.Snippet); snippet != "" {
			line += "\t" + snippet
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderProfileText(resp hubProfileResponse, category string) string {
	if category != "" {
		for _, profile := range resp.Profiles {
			if profile.Category == category {
				text := profile.Content
				if !strings.HasSuffix(text, "\n") {
					text += "\n"
				}
				return text
			}
		}
		return ""
	}
	lines := []string{
		fmt.Sprintf("slug: %s", resp.Slug),
		fmt.Sprintf("display_name: %s", resp.DisplayName),
		fmt.Sprintf("timezone: %s", resp.Timezone),
		fmt.Sprintf("language: %s", resp.Language),
	}
	for _, profile := range resp.Profiles {
		lines = append(lines, fmt.Sprintf("profile/%s", profile.Category))
	}
	return strings.Join(lines, "\n")
}

func renderProjectText(resp hubProjectResponse) string {
	lines := []string{
		fmt.Sprintf("name: %s", resp.Project.Name),
		fmt.Sprintf("status: %s", resp.Project.Status),
	}
	if strings.TrimSpace(resp.Project.ContextMD) != "" {
		lines = append(lines, "", resp.Project.ContextMD)
	}
	if len(resp.Logs) > 0 {
		lines = append(lines, "", "logs:")
		for _, log := range resp.Logs {
			lines = append(lines, fmt.Sprintf("- %s [%s] %s", log.CreatedAt.Format(time.RFC3339), log.Action, log.Summary))
		}
	}
	return strings.Join(lines, "\n")
}

func renderTokenText(resp hubTokenCreateResponse) string {
	lines := []string{
		fmt.Sprintf("token: %s", resp.Token),
		fmt.Sprintf("expires_at: %s", resp.ExpiresAt),
		fmt.Sprintf("api_base: %s", resp.APIBase),
	}
	if strings.TrimSpace(resp.UploadURL) != "" {
		lines = append(lines, fmt.Sprintf("upload_url: %s", resp.UploadURL))
	}
	if strings.TrimSpace(resp.BrowserUploadURL) != "" {
		lines = append(lines, fmt.Sprintf("browser_upload_url: %s", resp.BrowserUploadURL))
	}
	if strings.TrimSpace(resp.ConnectivityProbeURL) != "" {
		lines = append(lines, fmt.Sprintf("connectivity_probe_url: %s", resp.ConnectivityProbeURL))
	}
	if strings.TrimSpace(resp.CurlExample) != "" {
		lines = append(lines, fmt.Sprintf("curl_example: %s", resp.CurlExample))
	}
	if strings.TrimSpace(resp.Usage) != "" {
		lines = append(lines, fmt.Sprintf("usage: %s", resp.Usage))
	}
	if strings.TrimSpace(resp.Warning) != "" {
		lines = append(lines, fmt.Sprintf("warning: %s", resp.Warning))
	}
	return strings.Join(lines, "\n")
}

func renderKeyValueText(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s: %v", key, payload[key]))
	}
	return strings.Join(lines, "\n")
}

func renderImportText(category string, resp api.ImportResponse) string {
	return fmt.Sprintf("Imported %d %s item(s).", resp.Data.ImportedCount, category)
}
