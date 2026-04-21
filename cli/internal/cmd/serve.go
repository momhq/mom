package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/mcp"
	"github.com/momhq/mom/cli/internal/scope"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a server (use --mcp for MCP stdio mode)",
}

var serveMCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP stdio server",
	Long: `Start the MOM MCP (Model Context Protocol) server on stdio.

Any MCP-aware runtime (Claude Code, Cursor, Cline, …) can connect by adding
this command to its MCP config:

  {
    "mcpServers": {
      "mom": {
        "command": "leo",
        "args": ["serve", "mcp"]
      }
    }
  }

stdout is reserved for JSON-RPC — all human-readable output goes to stderr.
Block until stdin is closed or SIGINT.`,
	RunE: runServeMCP,
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show MCP server activity from the log",
	RunE:  runServerStatus,
}

func init() {
	serverStatusCmd.Flags().Int("lines", 20, "Number of recent log lines to show")
	serveCmd.AddCommand(serveMCPCmd)
	serveCmd.AddCommand(serverStatusCmd)
}

func runServeMCP(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return fmt.Errorf("no .mom/ directory found. Run 'mom init' first")
	}

	srv := mcp.New(sc.Path)
	// Blocks until stdin is closed.
	srv.Serve(os.Stdin, os.Stdout)
	return nil
}

func runServerStatus(cmd *cobra.Command, _ []string) error {
	lines, _ := cmd.Flags().GetInt("lines")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return fmt.Errorf("no .mom/ directory found. Run 'mom init' first")
	}

	logPath := filepath.Join(sc.Path, "logs", "mcp-server.log")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Println("No MCP server log found. The server has not been run yet.")
			return nil
		}
		return fmt.Errorf("opening log: %w", err)
	}
	defer f.Close()

	type logLine struct {
		ts     time.Time
		status string
		method string
		detail string
	}

	var allLines []logLine
	scanner := bufio.NewScanner(f)
	var errorCount int

	for scanner.Scan() {
		text := scanner.Text()
		parts := strings.Fields(text)
		if len(parts) < 3 {
			continue
		}
		ts, err := time.Parse(time.RFC3339, parts[0])
		if err != nil {
			continue
		}
		status := parts[1]
		method := parts[2]
		detail := ""
		if len(parts) > 3 {
			detail = strings.Join(parts[3:], " ")
		}
		allLines = append(allLines, logLine{ts: ts, status: status, method: method, detail: detail})
		if strings.EqualFold(status, "error") {
			errorCount++
		}
	}

	if len(allLines) == 0 {
		cmd.Println("Log file is empty.")
		return nil
	}

	// Last activity timestamp.
	lastActivity := allLines[len(allLines)-1].ts

	// Method call counts.
	methodCounts := make(map[string]int)
	for _, l := range allLines {
		methodCounts[l.method]++
	}

	cmd.Printf("Last activity : %s\n", lastActivity.Format(time.RFC3339))
	cmd.Printf("Error count   : %s\n", strconv.Itoa(errorCount))
	cmd.Printf("Total entries : %d\n", len(allLines))
	cmd.Println()

	// Print method counts sorted by count desc.
	type mc struct {
		method string
		count  int
	}
	var mcs []mc
	for m, c := range methodCounts {
		mcs = append(mcs, mc{m, c})
	}
	sort.Slice(mcs, func(i, j int) bool {
		if mcs[i].count != mcs[j].count {
			return mcs[i].count > mcs[j].count
		}
		return mcs[i].method < mcs[j].method
	})
	cmd.Println("Method counts:")
	for _, mc := range mcs {
		cmd.Printf("  %-30s %d\n", mc.method, mc.count)
	}
	cmd.Println()

	// Print last N lines.
	recent := allLines
	if len(recent) > lines {
		recent = recent[len(recent)-lines:]
	}
	cmd.Printf("Recent %d entries:\n", len(recent))
	for _, l := range recent {
		if l.detail != "" {
			cmd.Printf("  %s  %-6s  %-30s  %s\n", l.ts.Format(time.RFC3339), l.status, l.method, l.detail)
		} else {
			cmd.Printf("  %s  %-6s  %s\n", l.ts.Format(time.RFC3339), l.status, l.method)
		}
	}

	return nil
}
