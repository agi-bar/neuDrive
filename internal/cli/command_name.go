package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	canonicalRootCommand = "neudrive"
	shortRootCommand     = "neu"
)

func rootCommand() string {
	base := strings.TrimSpace(filepath.Base(os.Args[0]))
	switch base {
	case canonicalRootCommand, shortRootCommand:
		return base
	default:
		return canonicalRootCommand
	}
}

func usageLine(args string) string {
	return fmt.Sprintf("usage: %s %s", rootCommand(), args)
}

func renderCLIText(text string) string {
	cmd := rootCommand()
	if cmd == canonicalRootCommand {
		return text
	}
	replacer := strings.NewReplacer(
		"usage: "+canonicalRootCommand+" ", "usage: "+cmd+" ",
		"Usage: "+canonicalRootCommand+" ", "Usage: "+cmd+" ",
		"\n  "+canonicalRootCommand+" ", "\n  "+cmd+" ",
		"\n       "+canonicalRootCommand+" ", "\n       "+cmd+" ",
		"\n"+canonicalRootCommand+" ", "\n"+cmd+" ",
		"`"+canonicalRootCommand+" ", "`"+cmd+" ",
		" "+canonicalRootCommand+" help", " "+cmd+" help",
		" "+canonicalRootCommand+" ls", " "+cmd+" ls",
		" "+canonicalRootCommand+" read", " "+cmd+" read",
		" "+canonicalRootCommand+" write", " "+cmd+" write",
		" "+canonicalRootCommand+" search", " "+cmd+" search",
		" "+canonicalRootCommand+" create", " "+cmd+" create",
		" "+canonicalRootCommand+" log", " "+cmd+" log",
		" "+canonicalRootCommand+" import", " "+cmd+" import",
		" "+canonicalRootCommand+" token", " "+cmd+" token",
		" "+canonicalRootCommand+" stats", " "+cmd+" stats",
		" "+canonicalRootCommand+" git", " "+cmd+" git",
		" "+canonicalRootCommand+" platform", " "+cmd+" platform",
		" "+canonicalRootCommand+" connect", " "+cmd+" connect",
		" "+canonicalRootCommand+" disconnect", " "+cmd+" disconnect",
		" "+canonicalRootCommand+" export", " "+cmd+" export",
		" "+canonicalRootCommand+" browse", " "+cmd+" browse",
		" "+canonicalRootCommand+" status", " "+cmd+" status",
		" "+canonicalRootCommand+" doctor", " "+cmd+" doctor",
		" "+canonicalRootCommand+" daemon", " "+cmd+" daemon",
		" "+canonicalRootCommand+" sync", " "+cmd+" sync",
		" "+canonicalRootCommand+" remote", " "+cmd+" remote",
		" "+canonicalRootCommand+" server", " "+cmd+" server",
		" "+canonicalRootCommand+" mcp", " "+cmd+" mcp",
		"with "+canonicalRootCommand+" ", "with "+cmd+" ",
	)
	return replacer.Replace(text)
}
