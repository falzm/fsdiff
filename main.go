package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/falzm/fsdiff/internal/version"
)

func init() {
}

func main() {
	rootCmd := struct {
		Snapshot snapshotCmd `cmd:"" aliases:"snap" help:"Scan file tree and record object properties."`
		Diff     diffCmd     `cmd:"" help:"Show the differences between 2 snapshots."`
		Dump     dumpCmd     `cmd:"" help:"Dump snapshot information."`

		Version kong.VersionFlag `short:"v" help:"Print version information and quit."`
	}{}

	app := kong.Parse(
		&rootCmd,
		kong.Name("fsdiff"),
		kong.Description(
			"fsdiff reports what changes occurred in a filesystem tree.",
		),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:   true,
			FlagsLast: true,
		}),
		kong.UsageOnError(),
		kong.Vars{
			"diff_file_properties": strings.Join(diffFileProperties, ", "),
			"version": fmt.Sprintf(
				"fsdiff %s (commit: %s) %s\nbuild info: Go %s (%s)",
				version.Version,
				version.Commit,
				version.BuildDate,
				runtime.Version(),
				runtime.Compiler,
			),
		},
	)

	app.BindTo(*app, (*kong.Context)(nil))
	app.FatalIfErrorf(app.Run())
}
