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
		if err := doSnapshot(
			*cmdSnapshotArgRoot,
			*cmdSnapshotFlagOut,
			*cmdSnapshotFlagCarryOn,
			*cmdSnapshotFlagShallow); err != nil {
			dieOnError("%s", err)
		}

	case cmdDiff.FullCommand():
		if *cmdDiffFlagNoColor {
			ansi.DisableColors(true)
		}
		if err := doDiff(
			*cmdDiffArgSnapshotBefore,
			*cmdDiffArgSnapshotAfter,
			*cmdDiffFlagIgnore,
			*cmdDiffFlagSummary); err != nil {
			dieOnError("%s", err)
		}

	case cmdDump.FullCommand():
		if err := doDump(*cmdDumpArgSnapshot, *cmdDumpFlagMetadata); err != nil {
			dieOnError("%s", err)
		}
	}
}
