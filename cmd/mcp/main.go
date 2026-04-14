package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/agi-bar/neudrive/internal/app/mcpapp"
)

func main() {
	token := flag.String("token", "", "Scoped access token (ndt_...)")
	tokenEnv := flag.String("token-env", mcpapp.DefaultTokenEnvVar, "Environment variable name containing the scoped access token")
	flag.Parse()

	if _, err := mcpapp.ResolveToken(*token, *tokenEnv); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "usage: neudrive-mcp --token ndt_xxxxx\n")
		fmt.Fprintf(os.Stderr, "   or: export %s=ndt_xxxxx && neudrive-mcp --token-env %s\n", mcpapp.DefaultTokenEnvVar, mcpapp.DefaultTokenEnvVar)
		os.Exit(1)
	}

	if err := mcpapp.RunStdio(context.Background(), mcpapp.Options{
		Token:    *token,
		TokenEnv: *tokenEnv,
	}); err != nil {
		slog.Error("mcp stdio failed", "error", err)
		os.Exit(1)
	}
}
