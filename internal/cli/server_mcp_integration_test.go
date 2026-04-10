package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
	"github.com/agi-bar/agenthub/internal/storage/sqlite"
)

func TestAgenthubServerCommand_SQLite(t *testing.T) {
	binary := buildAgenthubBinary(t)
	listenAddr := findFreeListenAddr(t)
	sqlitePath := filepath.Join(t.TempDir(), "server.db")
	cmd := exec.Command(binary,
		"server",
		"--storage", "sqlite",
		"--sqlite-path", sqlitePath,
		"--listen", listenAddr,
		"--public-base-url", "http://"+listenAddr,
	)
	var stderr strings.Builder
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer stopTestProcess(t, cmd)

	apiBase := "http://" + listenAddr
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := waitForTestHealth(ctx, apiBase); err != nil {
		t.Fatalf("wait for health: %v\n%s", err, stderr.String())
	}
	resp, err := http.Get(apiBase + "/api/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("unexpected health status: %s", resp.Status)
	}
}

func TestAgenthubMCPStdio_SQLiteInitializeAndToolsList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio process signal handling differs on windows")
	}
	binary := buildAgenthubBinary(t)
	sqlitePath := filepath.Join(t.TempDir(), "mcp.db")
	cfg := &runtimecfg.CLIConfig{
		Version: 2,
		Local: runtimecfg.LocalConfig{
			Storage:        "sqlite",
			SQLitePath:     sqlitePath,
			Connections:    map[string]runtimecfg.LocalConnection{},
			JWTSecret:      strings.Repeat("a", 64),
			VaultMasterKey: strings.Repeat("b", 64),
		},
	}
	hub, err := sqlite.OpenClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open local hub: %v", err)
	}
	tokenResp, err := hub.CreateOwnerToken(context.Background())
	hub.Close()
	if err != nil {
		t.Fatalf("create owner token: %v", err)
	}

	cmd := exec.Command(binary,
		"mcp", "stdio",
		"--storage", "sqlite",
		"--sqlite-path", sqlitePath,
		"--token", tokenResp.Token,
		"--public-base-url", "http://127.0.0.1:42690",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp stdio: %v", err)
	}

	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"}); err != nil {
		t.Fatalf("encode initialize: %v", err)
	}
	if err := encoder.Encode(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tools/list"}); err != nil {
		t.Fatalf("encode tools/list: %v", err)
	}
	_ = stdin.Close()

	scanner := bufio.NewScanner(stdout)
	var responses []mcp.JSONRPCResponse
	for scanner.Scan() {
		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		responses = append(responses, resp)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan stdout: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait mcp stdio: %v\n%s", err, stderr.String())
	}
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Fatalf("initialize error: %+v", responses[0].Error)
	}
	if responses[1].Error != nil {
		t.Fatalf("tools/list error: %+v", responses[1].Error)
	}
	payload, err := json.Marshal(responses[1].Result)
	if err != nil {
		t.Fatalf("marshal tools/list result: %v", err)
	}
	if !strings.Contains(string(payload), "create_sync_token") {
		t.Fatalf("expected create_sync_token in tools/list result: %s", string(payload))
	}
	if !strings.Contains(string(payload), "prepare_skills_upload") {
		t.Fatalf("expected prepare_skills_upload in tools/list result: %s", string(payload))
	}
}

func findFreeListenAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitForTestHealth(ctx context.Context, apiBase string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := runtimecfg.HealthCheck(ctx, apiBase); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return context.DeadlineExceeded
}

func stopTestProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS != "windows" {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	} else {
		_ = cmd.Process.Kill()
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	case <-done:
	}
}
