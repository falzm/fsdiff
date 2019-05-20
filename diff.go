package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	fileProperties = []string{"size", "mtime", "uid", "gid", "mode", "checksum"}

	cmdDiff           = kingpin.Command("diff", "Show the differences between 2 snapshots")
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
func diff(before, after string, ignore []string, summary bool) error {
	var (
		nNew, nDeleted, nChanged int
		moved                    = make(map[string]struct{}) // Used to track file renamings
	)

	dbBefore, err := bolt.Open(before, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return errors.Wrap(err, `unable to open "before" snapshot file`)
	}
	defer dbBefore.Close()

	dbAfter, err := bolt.Open(after, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return errors.Wrap(err, `unable to open "after" snapshot file`)
	}
	defer dbAfter.Close()

	err = dbBefore.View(func(txB *bolt.Tx) error {
		pathBucketBefore := txB.Bucket([]byte("by_path"))
		if pathBucketBefore == nil {
			dieOnError(`"by_path" bucket not found in "before" snapshot file`)
		}

		csBucketBefore := txB.Bucket([]byte("by_cs"))
		if csBucketBefore == nil {
			dieOnError(`"by_cs" bucket not found in "before" snapshot file`)
		}

		err = dbAfter.View(func(txA *bolt.Tx) error {
			pathBucketAfter := txA.Bucket([]byte("by_path"))
			if pathBucketAfter == nil {
				dieOnError(`"by_path" bucket not found in "after" snapshot file`)
			}

			if err := pathBucketAfter.ForEach(func(path, data []byte) error {
				fileInfoAfter := fileInfo{}
				unmarshal(data, &fileInfoAfter)

				if beforeData := pathBucketBefore.Get(path); beforeData != nil {
					// The file existed before, check that its properties have changed
					fileInfoBefore := fileInfo{}
					unmarshal(beforeData, &fileInfoBefore)

					fileDiff := compare(&fileInfoBefore, &fileInfoAfter, ignore)
					if len(fileDiff) > 0 {
						if !summary {
							printChanged(&fileInfoBefore, &fileInfoAfter, fileDiff)
						}
						nChanged++
					}
					return nil
				}

				// No file existed before at this path, check by checksum to see if it's a previous file moved
				// elsewhere. We skip empty files since they cause false positives due to identical checksum.
				if fileInfoAfter.Size > 0 {
					if beforeData := csBucketBefore.Get(fileInfoAfter.Checksum); beforeData != nil {
						// The file existed before elsewhere, check that its properties have changed
						fileInfoBefore := fileInfo{}
						unmarshal(beforeData, &fileInfoBefore)

						moved[fileInfoBefore.Path] = struct{}{}

						fileDiff := compare(&fileInfoBefore, &fileInfoAfter, ignore)
						if !summary {
							printChanged(&fileInfoBefore, &fileInfoAfter, fileDiff)
						}
						nChanged++
						return nil
					}
				}

				// No file match this checksum, this is a new file
				if !summary {
					printNew(string(path))
				}
				nNew++
				return nil
			}); err != nil {
				dieOnError("bolt: unable to loop on bucket keys: %s", err)
			}

			// Perform reverse lookup to detect deleted files
			if err := pathBucketBefore.ForEach(func(path, data []byte) error {
				if afterData := pathBucketAfter.Get(path); afterData == nil {
					// Before marking a file as deleted, check if it is not the result of a renaming
					if _, ok := moved[string(path)]; !ok {
						if !summary {
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
		if !summary {
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
