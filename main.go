package main

import (
	"fmt"
	"os"
	"runtime"

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
	var err error

	switch kingpin.Parse() {
	case cmdSnapshot.FullCommand():
		err = doSnapshot()

	case cmdDiff.FullCommand():
		err = doDiff()

	case cmdDump.FullCommand():
		err = doDump()
	}

	if err != nil {
		dieOnError("%s", err)
	}
}
