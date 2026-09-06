// Package migratecli adapts command-line arguments and JSON output to the
// migration application. It constructs no source or target infrastructure.
package migratecli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
)

type Command struct{ Verb, PlanPath, WorkspacePath, ArchivePath string }
type Execute func(context.Context, Command) (any, error)

// Run emits machine-readable results on stdout and diagnostics on stderr.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, execute Execute) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(stdout, "Usage: wkmigrate <prepare|export|import|verify> --plan /path/plan.json --workspace /path/workspace [--archive /path/archive]\n\nprepare  Inspect stopped original v2 nodes; select authoritative data and validate conversion.\nexport   Recheck unchanged stopped sources and publish a checksummed source archive.\nimport   Install a new native v3 cluster generation from the complete archive.\nverify   Independently compare stopped v3 databases with archived original data.\n\nNo command modifies or upgrades deployed v2. A prepared or exported result is not cutover approval.")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	cmd := Command{Verb: args[0]}
	if cmd.Verb != "prepare" && cmd.Verb != "export" && cmd.Verb != "import" && cmd.Verb != "verify" {
		fmt.Fprintf(stderr, "unknown migration command %q\n", cmd.Verb)
		return 2
	}
	flags := flag.NewFlagSet("wkmigrate "+cmd.Verb, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cmd.PlanPath, "plan", "", "immutable migration plan JSON")
	flags.StringVar(&cmd.WorkspacePath, "workspace", "", "exclusive migration scratch directory")
	flags.StringVar(&cmd.ArchivePath, "archive", "", "source archive directory (required for export, import and verify)")
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || cmd.PlanPath == "" || cmd.WorkspacePath == "" || (cmd.Verb != "prepare" && cmd.ArchivePath == "") {
		fmt.Fprintln(stderr, "migration requires --plan, --workspace, and --archive for export/import/verify")
		return 2
	}
	result, err := execute(ctx, cmd)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
