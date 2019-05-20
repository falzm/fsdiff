package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/mgutz/ansi"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	version   string
	commit    string
	buildDate string

	cmdSnapshot = kingpin.Command("snapshot", "Scan file tree and record object properties").
			Alias("snap")
)

func dieOnError(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, fmt.Sprintf("error: %s\n", format), a...)
	os.Exit(1)
}

func init() {
	kingpin.Version(fmt.Sprintf("fsdiff %s (commit: %s) %s\nbuild info: Go %s (%s)",
		version,
		commit,
		buildDate,
		runtime.Version(),
		runtime.Compiler))
}

func main() {
	switch kingpin.Parse() {
	case cmdSnapshot.FullCommand():
		if err := snapshot(
			*cmdSnapshotArgRoot,
			*cmdSnapshotFlagOut,
			*cmdSnapshotFlagCarryOn,
			*cmdSnapshotFlagShallow); err != nil {
			dieOnError("unable to snapshot filesystem: %s", err)
		}

	case cmdDiff.FullCommand():
		if *cmdDiffFlagNoColor {
			ansi.DisableColors(true)
		}
		if err := diff(
			*cmdDiffArgSnapshotBefore,
			*cmdDiffArgSnapshotAfter,
			*cmdDiffFlagIgnore,
			*cmdDiffFlagSummary); err != nil {
			dieOnError("%s", err)
		}

	case cmdDump.FullCommand():
		if err := dump(*cmdDumpArgSnapshot); err != nil {
			dieOnError("%s", err)
		}
	}
}
