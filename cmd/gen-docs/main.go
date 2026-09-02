// Command gen-docs generates CLI reference documentation for the `er` command
// tree. It walks the Cobra commands and emits one Markdown file per command.
//
// Two modes are supported:
//
//	--standard  plain Markdown (portable, e.g. for GitHub browsing)
//	--custom    Hugo-friendly Markdown with front matter (title, weight)
//
// It intentionally depends only on the standard library and Cobra (already a
// project dependency) so documentation generation adds no new modules.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	ercmd "github.com/oakwood-commons/event-reactor/pkg/cmd"
)

func main() {
	var (
		docPath  string
		standard bool
		custom   bool
	)

	flag.StringVar(&docPath, "doc-path", "docs/event-reactor", "output directory for generated docs")
	flag.BoolVar(&standard, "standard", false, "generate plain Markdown")
	flag.BoolVar(&custom, "custom", false, "generate Hugo-flavored Markdown with front matter")
	flag.Parse()

	if standard == custom {
		fmt.Fprintln(os.Stderr, "error: specify exactly one of --standard or --custom")
		os.Exit(2)
	}

	root := ercmd.NewRootCmd()
	root.DisableAutoGenTag = true

	if err := run(root, docPath, custom); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(root *cobra.Command, docPath string, hugo bool) error {
	if err := os.MkdirAll(docPath, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// weight is a monotonically increasing counter used only in Hugo mode to
	// preserve command-tree ordering in the rendered nav.
	weight := 0
	return walk(root, docPath, hugo, &weight)
}

func walk(cmd *cobra.Command, docPath string, hugo bool, weight *int) error {
	if !cmd.IsAvailableCommand() && cmd.Name() != "" && cmd.HasParent() {
		return nil
	}

	(*weight)++
	if err := writeDoc(cmd, docPath, hugo, *weight); err != nil {
		return err
	}

	children := cmd.Commands()
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name() < children[j].Name()
	})
	for _, child := range children {
		if child.Name() == "help" || child.Hidden {
			continue
		}
		if err := walk(child, docPath, hugo, weight); err != nil {
			return err
		}
	}
	return nil
}

func writeDoc(cmd *cobra.Command, docPath string, hugo bool, weight int) error {
	fileName := strings.ReplaceAll(cmd.CommandPath(), " ", "_") + ".md"
	path := filepath.Join(docPath, fileName)

	var b strings.Builder
	if hugo {
		fmt.Fprintf(&b, "---\ntitle: %q\nweight: %d\n---\n\n", cmd.CommandPath(), weight)
	}

	fmt.Fprintf(&b, "# %s\n\n", cmd.CommandPath())

	if cmd.Long != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(cmd.Long))
	} else if cmd.Short != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(cmd.Short))
	}

	if cmd.Runnable() {
		fmt.Fprintf(&b, "## Usage\n\n```\n%s\n```\n\n", cmd.UseLine())
	}

	if flags := strings.TrimRight(cmd.LocalFlags().FlagUsages(), "\n"); flags != "" {
		fmt.Fprintf(&b, "## Flags\n\n```\n%s\n```\n\n", flags)
	}
	if flags := strings.TrimRight(cmd.InheritedFlags().FlagUsages(), "\n"); flags != "" {
		fmt.Fprintf(&b, "## Global Flags\n\n```\n%s\n```\n\n", flags)
	}

	var subs []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "help" || c.Hidden || !c.IsAvailableCommand() {
			continue
		}
		subs = append(subs, c)
	}
	if len(subs) > 0 {
		sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
		fmt.Fprint(&b, "## Subcommands\n\n")
		for _, c := range subs {
			link := strings.ReplaceAll(c.CommandPath(), " ", "_") + ".md"
			fmt.Fprintf(&b, "- [%s](%s) -- %s\n", c.CommandPath(), link, c.Short)
		}
		b.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
