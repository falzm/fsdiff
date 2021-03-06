package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/falzm/fsdiff/snapshot"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/src-d/go-git.v4/plumbing/format/gitignore"
)

var (
	fileProperties = []string{"size", "mtime", "uid", "gid", "mode", "checksum"}

	cmdDiff            = kingpin.Command("diff", "Show the differences between 2 snapshots")
	cmdDiffFlagExclude = cmdDiff.Flag("exclude",
		"gitignore-compatible exclusion pattern (see https://git-scm.com/docs/gitignore)").
		PlaceHolder("PATTERN").
		Strings()
	cmdDiffFlagIgnore = cmdDiff.Flag("ignore", fmt.Sprintf("Ignore file properties (%s)",
		strings.Join(fileProperties, ", "))).
		PlaceHolder("PROPERTY").
		Enums(fileProperties...)
	cmdDiffFlagNoColor = cmdDiff.Flag("nocolor", "Disable output coloring").
				Bool()
	cmdDiffFlagSummary = cmdDiff.Flag("summary", "Display only changes summary").
				Bool()
	cmdDiffArgSnapshotBefore = cmdDiff.Arg("before", "Path to \"before\" snapshot file").
					Required().
					ExistingFile()
	cmdDiffArgSnapshotAfter = cmdDiff.Arg("after", "Path to \"after\" snapshot file").
				Required().
				ExistingFile()
)

/*
	The diff logic is implemented as follows:

	1) For each file in _after_ snapshot, check if it existed at the same path in the _before_ snapshot:
	   - if it existed, check if its properties match the _before_ ones
	     * if they don't, mark the file [modified]
	   - if it didnt, check if a file with a matching checksum existed at a different path:
	     * if found, mark the file [moved] and check if its properties match the _before_ ones
	     * if none found, mark the file [new]

	2) For each file in _before_ snapshot, check if it exists in the *after* snapshot:
	   - if it doesn't, mark the file [deleted]
*/
func doDiff() error {
	var (
		before      = *cmdDiffArgSnapshotBefore
		after       = *cmdDiffArgSnapshotAfter
		exclude     = *cmdDiffFlagExclude
		ignore      = *cmdDiffFlagIgnore
		summaryOnly = *cmdDiffFlagSummary

		nNew, nDeleted, nChanged int
		shallow                  bool
		moved                    = make(map[string]struct{}) // Used to track file renamings
		excludedPatterns         []gitignore.Pattern
		excluded                 gitignore.Matcher
	)

	if *cmdDiffFlagNoColor {
		ansi.DisableColors(true)
	}

	for _, p := range exclude {
		excludedPatterns = append(excludedPatterns, gitignore.ParsePattern(p, nil))
	}
	excluded = gitignore.NewMatcher(excludedPatterns)

	snapBefore, err := snapshot.Open(before)
	if err != nil {
		return errors.Wrap(err, `unable to open "before" snapshot file`)
	}
	defer snapBefore.Close()

	snapAfter, err := snapshot.Open(after)
	if err != nil {
		return errors.Wrap(err, `unable to open "after" snapshot file`)
	}
	defer snapAfter.Close()

	snapBefore.Read(func(byPathBefore, byCSBefore *bolt.Bucket) error {
		snapAfter.Read(func(byPathAfter, byCSAfter *bolt.Bucket) error {
			// If either one of the before/after snapshots is shallow, diff in shallow mode
			if snapBefore.Metadata().Shallow || snapAfter.Metadata().Shallow {
				shallow = true
			}

			byPathAfter.ForEach(func(path, data []byte) error {
				fileInfoAfter := fileInfo{}
				if err := snapshot.Unmarshal(data, &fileInfoAfter); err != nil {
					dieOnError("unable to read snapshot data: %s", err)
				}

				// Skip files matching the excluded patterns
				if excluded.Match(strings.Split(fileInfoAfter.Path, "/"), fileInfoAfter.IsDir) {
					return nil
				}

				if beforeData := byPathBefore.Get(path); beforeData != nil {
					// The file existed before, check that its properties have changed
					fileInfoBefore := fileInfo{}
					if err := snapshot.Unmarshal(beforeData, &fileInfoBefore); err != nil {
						dieOnError("unable to read snapshot data: %s", err)
					}

					fileDiff := compare(&fileInfoBefore, &fileInfoAfter, ignore)
					if len(fileDiff) > 0 {
						if !summaryOnly {
							printChanged(&fileInfoBefore, &fileInfoAfter, fileDiff)
						}
						nChanged++
					}
					return nil
				}

				// No file existed before at this path, check by checksum to see if it's a previous file moved
				// elsewhere -- unless we're in shallow mode, since we don't have the files checksum.
				// We skip empty files since they cause false positives due to identical checksum.
				if fileInfoAfter.Size > 0 && !shallow {
					if beforeData := byCSBefore.Get(fileInfoAfter.Checksum); beforeData != nil {
						// The file existed before elsewhere, check that its properties have changed
						fileInfoBefore := fileInfo{}
						if err := snapshot.Unmarshal(beforeData, &fileInfoBefore); err != nil {
							dieOnError("unable to read snapshot data: %s", err)
						}

						moved[fileInfoBefore.Path] = struct{}{}

						fileDiff := compare(&fileInfoBefore, &fileInfoAfter, ignore)
						if !summaryOnly {
							printChanged(&fileInfoBefore, &fileInfoAfter, fileDiff)
						}
						nChanged++
						return nil
					}
				}

				// No file match this checksum, this is a new file
				if !summaryOnly {
					printNew(string(path))
				}
				nNew++
				return nil
			})

			// Perform reverse lookup to detect deleted files
			if err := byPathBefore.ForEach(func(path, data []byte) error {
				if afterData := byPathAfter.Get(path); afterData == nil {
					// Before marking a file as deleted, check if it is not the result of a renaming
					if _, ok := moved[string(path)]; !ok {
						if !summaryOnly {
							printDeleted(string(path))
						}
						nDeleted++
					}
				}

				return nil
			}); err != nil {
				dieOnError("bolt: unable to loop on bucket keys: %s", err)
			}

			return nil
		})

		return nil
	})

	if nNew > 0 || nChanged > 0 || nDeleted > 0 {
		if !summaryOnly {
			fmt.Println()
		}
		fmt.Printf("%d new, %d changed, %d deleted\n",
			nNew,
			nChanged,
			nDeleted)

		os.Exit(2)
	}

	return nil
}

func printNew(f string) {
	fmt.Println(ansi.Color("+", "green"), f)
}

func printChanged(before, after *fileInfo, diff map[string][2]interface{}) {
	if before.Path != after.Path {
		fmt.Printf("%s %s => %s\n", ansi.Color(">", "cyan"), before.Path, after.Path)
	} else {
		fmt.Printf("%s %s\n", ansi.Color("~", "yellow"), after.Path)
	}

	if len(diff) > 0 {
		fmt.Printf("  %s\n  %s\n",
			before.String(),
			after.String())
	}
}

func printDeleted(f string) {
	fmt.Println(ansi.Color("-", "red"), f)
}
