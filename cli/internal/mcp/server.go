// Package mcp implements a minimal Model Context Protocol server over stdio.
// It exposes MOM memories as MCP tools and resources, allowing any MCP-aware
// runtime (Claude Code, Cursor, Cline, …) to query and write memories without
// adapter code.
//
// Transport: JSON-RPC 2.0, newline-delimited, over stdin/stdout.
// stdout is reserved for JSON-RPC — all human-readable output goes to stderr.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/recall"
	"github.com/momhq/mom/cli/internal/store"
	"github.com/momhq/mom/cli/internal/ux"
	"github.com/momhq/mom/cli/internal/vault"
)

// Version is set by the caller (cmd package) to match the CLI version.
var Version = "dev"

const (
	// MCPProtocolVersion is the MCP spec version this server implements.
	MCPProtocolVersion = "2024-11-05"

	// JSON-RPC error codes (subset of spec).
	errCodeParseError     = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternalError  = -32603
)

// jsonRPCRequest is an inbound JSON-RPC 2.0 message.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"` // string | number | null; absent for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is an outbound JSON-RPC 2.0 message.
type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server is the MCP server instance.
type Server struct {
	momDir      string
	idx         *storage.IndexedAdapter
	engine      *recall.Engine
	vault       *vault.Vault
	memoryStore *store.MemoryStore
	graphStore  *store.GraphStore
}

// SetVault wires the v0.30 Vault into the Server and constructs all
// vault-backed components (recall.Engine, store.MemoryStore,
// store.GraphStore) so handlers can call them directly without
// re-wrapping the vault on every call. Required by tools that target
// the central vault (mom_record, mom_recall, mom_get, mom_landmarks).
// Production callers always set this in serve.go; tests for
// legacy-only tools may skip it.
func (s *Server) SetVault(v *vault.Vault) {
	s.vault = v
	s.engine = recall.NewEngine(v)
	s.memoryStore = store.NewMemoryStore(v)
	s.graphStore = store.NewGraphStore(v)
}

// New creates a new Server rooted at the given .mom/ directory.
// The recall Engine is left nil until SetVault is called.
func New(momDir string) *Server {
	idx := storage.NewIndexedAdapter(momDir)

	return &Server{
		momDir: momDir,
		idx:    idx,
	}
}

// Serve runs the JSON-RPC 2.0 stdio loop. It reads newline-delimited requests
// from in and writes responses to out. Blocks until in is closed or returns an
// unrecoverable read error.
//
// stdout (out) is reserved for JSON-RPC only. Human-readable output goes to
// stderr.
func (s *Server) Serve(in io.Reader, out io.Writer) {
	p := ux.NewPrinter(os.Stderr)
	p.Diamond("MCP server")
	p.Chevron(fmt.Sprintf("scope: %s", s.momDir))
	p.Muted("listening on stdio — stdout reserved for JSON-RPC")
	p.Blank()

	// Open log file in append mode.
	logFile := s.openLog()
	if logFile != nil {
		defer logFile.Close()
	}

	enc := json.NewEncoder(out)
	scanner := bufio.NewScanner(in)
	// Increase buffer for large requests.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.logEntry(logFile, "parse_error", string(line), err.Error())
			_ = enc.Encode(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &rpcError{Code: errCodeParseError, Message: "parse error: " + err.Error()},
			})
			continue
		}

		// Notifications have no id — do not send a response.
		if req.ID == nil && req.Method != "" {
			s.logEntry(logFile, "notification", req.Method, "")
			continue
		}

		result, rpcErr := s.dispatch(req.Method, req.Params)
		if rpcErr != nil {
			s.logEntry(logFile, "error", req.Method, rpcErr.Message)
			p.Failf("%s — %s", req.Method, rpcErr.Message)
			_ = enc.Encode(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   rpcErr,
			})
		} else {
			s.logEntry(logFile, "ok", req.Method, "")
			p.Checkf("%s", req.Method)
			_ = enc.Encode(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			})
		}
	}
}

// dispatch routes a JSON-RPC method to its handler.
func (s *Server) dispatch(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "tools/list":
		return s.handleToolsList()
	case "tools/call":
		return s.handleToolsCall(params)
	case "resources/list":
		return s.handleResourcesList()
	case "resources/read":
		return s.handleResourcesRead(params)
	default:
		return nil, &rpcError{Code: errCodeMethodNotFound, Message: "method not found: " + method}
	}
}

// handleInitialize processes the MCP initialize handshake.
func (s *Server) handleInitialize(_ json.RawMessage) (any, *rpcError) {
	return map[string]any{
		"protocolVersion": MCPProtocolVersion,
		"capabilities": map[string]any{
			"tools":     map[string]any{},
			"resources": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "mom-mcp-server",
			"version": Version,
		},
	}, nil
}

// openLog opens (or creates) the MCP server log file in append mode.
// Returns nil on failure so the caller can handle nil gracefully.
func (s *Server) openLog() *os.File {
	logDir := s.momDir + "/logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil
	}
	f, err := os.OpenFile(logDir+"/mcp-server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	return f
}

// logEntry writes a single log line with timestamp, status, method, and detail.
func (s *Server) logEntry(f *os.File, status, method, detail string) {
	if f == nil {
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	if detail != "" {
		fmt.Fprintf(f, "%s  %-6s  %s  %s\n", ts, status, method, detail)
	} else {
		fmt.Fprintf(f, "%s  %-6s  %s\n", ts, status, method)
	}
}
