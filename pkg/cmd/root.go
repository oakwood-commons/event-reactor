package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/oakwood-commons/event-reactor/pkg/adapter"
	"github.com/oakwood-commons/event-reactor/pkg/api"
	"github.com/oakwood-commons/event-reactor/pkg/auth"
	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/listener"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/mcp"
	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/observability"
	"github.com/oakwood-commons/event-reactor/pkg/params/version"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
	"github.com/oakwood-commons/event-reactor/pkg/reactor/providers"
)

// Execute runs the root command.
func Execute() error {
	return rootCmd().Execute()
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "er",
		Short:   "event-reactor -- event-driven automation engine",
		Version: version.String(),
	}

	cmd.AddCommand(versionCmd())
	cmd.AddCommand(runCmd())
	cmd.AddCommand(testCmd())
	cmd.AddCommand(mcpCmd())

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(version.String())
			return nil
		},
	}
}

func runCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run event-reactor components",
	}

	cmd.AddCommand(serverCmd())
	return cmd
}

func serverCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the event-reactor server",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServer(configFile)
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "path to server config file (required)")
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func runServer(configFile string) error {
	// Load config
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Set up logger
	logger := observability.Logger(cfg.Observability.Logging)

	// Set up matcher
	m, err := matcher.New()
	if err != nil {
		return fmt.Errorf("creating matcher: %w", err)
	}

	// Pre-compile all match expressions
	for _, rc := range cfg.Reactors {
		if rc.Match != "" {
			if _, err := m.Compile(rc.Match); err != nil {
				return fmt.Errorf("compiling match expression for reactor %q: %w", rc.Name, err)
			}
		}
	}

	// Set up registry (no providers registered yet -- will be wired later)
	reg := reactor.NewRegistry()
	providers.RegisterAll(reg, logger)

	// Set up adapter
	a := adapter.New(cfg, m, reg, logger)

	// Set up auth handlers
	if len(cfg.Auth.Handlers) > 0 {
		authReg, err := auth.NewRegistry(cfg.Auth.Handlers)
		if err != nil {
			return fmt.Errorf("initializing auth: %w", err)
		}
		a.WithAuth(authReg)
	}

	// Build background listeners from config. Listener types served by the HTTP
	// server (e.g. webhook) return nil here and are handled by srv below.
	var listeners []listener.Listener
	for _, lc := range cfg.Listeners {
		l, err := listener.Build(lc, logger)
		if err != nil {
			return fmt.Errorf("building listener %q: %w", lc.Name, err)
		}
		if l == nil {
			continue
		}
		listeners = append(listeners, l)
	}

	// Set up HTTP server
	srv := api.New(cfg, a, logger)

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting event-reactor",
		slog.String("version", version.String()),
		slog.Int("port", cfg.Server.Port),
		slog.Int("listeners", len(listeners)),
	)

	// Run the HTTP server and all background listeners concurrently. The first
	// fatal error cancels the shared context, shutting the others down cleanly.
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return srv.Start(ctx)
	})
	for _, l := range listeners {
		g.Go(func() error {
			return l.Start(ctx, func(ctx context.Context, ev message.Event) {
				a.HandleEvent(ctx, ev)
			})
		})
	}

	return g.Wait()
}

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server (Model Context Protocol) over stdio",
		RunE: func(_ *cobra.Command, _ []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
			reg := reactor.NewRegistry()
			srv := mcp.New(reg, logger)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			logger.Info("starting MCP server over stdio")
			return srv.ServeStdio(ctx, os.Stdin, os.Stdout, version.String())
		},
	}
}
