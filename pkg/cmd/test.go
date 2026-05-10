package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
	ertmpl "github.com/oakwood-commons/event-reactor/pkg/template"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test event-reactor components locally",
	}

	cmd.AddCommand(testMatchCmd())
	cmd.AddCommand(testTemplateCmd())
	cmd.AddCommand(testConfigCmd())
	cmd.AddCommand(testReactorCmd())

	return cmd
}

// ── er test match ──────────────────────────────────────────────────

func testMatchCmd() *cobra.Command {
	var eventFile string

	cmd := &cobra.Command{
		Use:   "match [cel-expression]",
		Short: "Test a CEL expression against an event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expr := args[0]

			event, err := loadEventFile(eventFile)
			if err != nil {
				return err
			}

			m, err := matcher.New()
			if err != nil {
				return fmt.Errorf("creating matcher: %w", err)
			}

			matched, err := m.Match(expr, event)
			if err != nil {
				return fmt.Errorf("evaluating expression: %w", err)
			}

			if matched {
				cmd.Println("✓ MATCH")
			} else {
				cmd.Println("✗ NO MATCH")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&eventFile, "event", "e", "", "path to event JSON file (required)")
	_ = cmd.MarkFlagRequired("event")

	return cmd
}

// ── er test template ───────────────────────────────────────────────

func testTemplateCmd() *cobra.Command {
	var (
		eventFile    string
		templateFile string
		templateStr  string
	)

	cmd := &cobra.Command{
		Use:   "template",
		Short: "Render a Go template against an event",
		RunE: func(cmd *cobra.Command, args []string) error {
			tmpl, err := resolveTemplate(templateStr, templateFile)
			if err != nil {
				return err
			}

			event, err := loadEventFile(eventFile)
			if err != nil {
				return err
			}

			result, err := ertmpl.Render(tmpl, event.AsMap())
			if err != nil {
				return fmt.Errorf("rendering template: %w", err)
			}

			cmd.Print(result)
			return nil
		},
	}

	cmd.Flags().StringVarP(&eventFile, "event", "e", "", "path to event JSON file (required)")
	cmd.Flags().StringVarP(&templateFile, "file", "f", "", "path to template file")
	cmd.Flags().StringVarP(&templateStr, "template", "t", "", "template string (inline)")
	_ = cmd.MarkFlagRequired("event")

	return cmd
}

// ── er test config ─────────────────────────────────────────────────

func testConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config [config-file]",
		Short: "Validate a server config file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(args[0])
			if err != nil {
				return err
			}

			cmd.Printf("✓ Config valid: %d listener(s), %d reactor(s)\n",
				len(cfg.Listeners), len(cfg.Reactors))

			// Also pre-compile all CEL expressions
			m, merr := matcher.New()
			if merr != nil {
				return fmt.Errorf("creating matcher: %w", merr)
			}

			for _, rc := range cfg.Reactors {
				if rc.Match != "" {
					if _, cerr := m.Compile(rc.Match); cerr != nil {
						return fmt.Errorf("reactor %q: invalid match expression: %w", rc.Name, cerr)
					}
				}
			}

			cmd.Println("✓ All CEL expressions valid")
			return nil
		},
	}
}

// ── er test reactor ────────────────────────────────────────────────

func testReactorCmd() *cobra.Command {
	var (
		configFile string
		eventFile  string
		reactorName string
		dryRun     bool
	)

	cmd := &cobra.Command{
		Use:   "reactor",
		Short: "Dry-run or execute a reactor against an event",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configFile)
			if err != nil {
				return err
			}

			event, err := loadEventFile(eventFile)
			if err != nil {
				return err
			}

			m, err := matcher.New()
			if err != nil {
				return fmt.Errorf("creating matcher: %w", err)
			}

			var rc *config.ReactorConfig
			for i := range cfg.Reactors {
				if cfg.Reactors[i].Name == reactorName {
					rc = &cfg.Reactors[i]
					break
				}
			}
			if rc == nil {
				return fmt.Errorf("reactor %q not found in config", reactorName)
			}

			// Check match
			matched, err := m.Match(rc.Match, event)
			if err != nil {
				return fmt.Errorf("evaluating match: %w", err)
			}

			if !matched {
				cmd.Println("✗ Event does not match reactor expression")
				return nil
			}
			cmd.Println("✓ Event matches reactor expression")

			// Resolve inputs
			resolved, err := reactor.ResolveInputs(*rc, event, m)
			if err != nil {
				return fmt.Errorf("resolving inputs: %w", err)
			}

			if dryRun {
				cmd.Println("\n── Resolved Inputs (dry-run) ──")
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(resolved)
			}

			cmd.Printf("Resolved %d input(s), provider=%s\n", len(resolved), rc.Provider)
			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "path to server config file (required)")
	cmd.Flags().StringVarP(&eventFile, "event", "e", "", "path to event JSON file (required)")
	cmd.Flags().StringVarP(&reactorName, "name", "n", "", "reactor name to test (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "resolve inputs without executing")
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("event")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// ── Helpers ────────────────────────────────────────────────────────

func loadEventFile(path string) (message.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return message.Event{}, fmt.Errorf("reading event file %s: %w", path, err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return message.Event{}, fmt.Errorf("parsing event JSON: %w", err)
	}

	return message.FromGenericPayload(payload)
}

func resolveTemplate(inline, file string) (string, error) {
	if inline != "" {
		return inline, nil
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading template file %s: %w", file, err)
		}
		return string(data), nil
	}
	return "", fmt.Errorf("either --template or --file is required")
}
