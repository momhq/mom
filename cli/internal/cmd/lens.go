package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/momhq/mom/cli/internal/lens"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
	"github.com/spf13/cobra"
)

const defaultLensPort = 7474

var lensCmd = &cobra.Command{
	Use:   "lens",
	Short: "Open the session history dashboard in your browser",
	Long: `Launch a local web server and open the MOM sessions dashboard.

The dashboard shows sessions across all .mom/ scopes visible from the current
directory. Use the scope switcher in the UI to filter by repo, org, or user.

Press Ctrl+C to stop the server.`,
	RunE: runLens,
}

func init() {
	lensCmd.Flags().Int("port", defaultLensPort, "Port to listen on")
}

func runLens(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Flags().GetInt("port")
	portExplicit := cmd.Flags().Changed("port")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		return fmt.Errorf("no .mom/ directory found — run 'mom init' first")
	}

	entries := make([]lens.ScopeEntry, len(scopes))
	for i, s := range scopes {
		entries[i] = lens.ScopeEntry{Label: s.Label, Path: s.Path}
	}

	srv, err := lens.New(entries)
	if err != nil {
		return fmt.Errorf("starting lens: %w", err)
	}

	// Default port: try up to 10 fallbacks. Explicit --port: fail loud if taken.
	fallbacks := 10
	if portExplicit {
		fallbacks = 0
	}
	ln, err := lens.ListenWithFallback("", port, fallbacks)
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", port, err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	url := fmt.Sprintf("http://localhost:%d", actualPort)
	p := ux.NewPrinter(cmd.OutOrStdout())
	p.Checkf("mom lens → %s", url)
	for _, s := range scopes {
		p.KeyValue("  scope", s.Label, 8)
	}
	p.Blank()
	p.Muted("  Ctrl+C to stop")

	_ = openBrowser(url)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Close()
	}()

	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}

	p.Blank()
	p.Muted("lens closed.")
	return nil
}
