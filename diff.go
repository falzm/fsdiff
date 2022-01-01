package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/mgutz/ansi"
	bolt "go.etcd.io/bbolt"
	"gopkg.in/src-d/go-git.v4/plumbing/format/gitignore"

	"github.com/falzm/fsdiff/internal/snapshot"
)

const (
	diffTypeNew = iota
	diffTypeModified
	diffTypeDeleted
)

type fileDiff struct {
	diffType   int
	fileBefore *snapshot.FileInfo
	fileAfter  *snapshot.FileInfo
	changes    map[string][2]interface{}
}

type diffCmdOutput struct {
	summary struct {
		new      int
		modified int
		deleted  int
	}
	changes []fileDiff
}

type diffCmd struct {
	Before string `arg:"" type:"existingfile" help:"Path to \"before\" snapshot file."`
	After  string `arg:"" type:"existingfile" help:"Path to \"after\" snapshot file."`

	Exclude        []string `placeholder:"PATTERN" help:"gitignore-compatible exclusion pattern (see https://git-scm.com/docs/gitignore)."`
	Ignore         []string `placeholder:"PROPERTY" enum:"${diff_file_properties}" help:"File property to ignore (${diff_file_properties})."`
	IgnoreNew      bool     `help:"Ignore any new file."`
	IgnoreModified bool     `help:"Ignore any modified file."`
	IgnoreDeleted  bool     `help:"Ignore any deleted file."`
	NoColor        bool     `name:"nocolor" help:"Disable output coloring."`
	Quiet          bool     `short:"q" help:"Disable any output.'"`
	SummaryOnly    bool     `name:"summary" help:"Only display changes summary."`
}

func (c *diffCmd) Help() string {
	return `Similar to the traditional "diff" tool, this command's exit
status has a specific meaning: 0 means no differences were found, 1 means
some differences were found, and 2 means trouble.`
}

var diffFileProperties = []string{
	"size",
	"mtime",
	"uid",
	"gid",
	"mode",
	"checksum",
}

func (c *diffCmd) run() (diffCmdOutput, error) {
	var (
		moved   = make(map[string]struct{}) // Used to track file renamings.
		shallow bool
	)

	excludedPatterns := make([]gitignore.Pattern, len(c.Exclude))
	for i, p := range c.Exclude {
		excludedPatterns[i] = gitignore.ParsePattern(p, nil)
	}
	excluded := gitignore.NewMatcher(excludedPatterns)

	snapBefore, err := snapshot.Open(c.Before)
	if err != nil {
		return diffCmdOutput{}, fmt.Errorf(`unable to open "before" snapshot file: %w`, err)
	}
	defer snapBefore.Close()

	snapAfter, err := snapshot.Open(c.After)
	if err != nil {
		return diffCmdOutput{}, fmt.Errorf(`unable to open "after" snapshot file: %w`, err)
	}
	defer snapAfter.Close()

	out := diffCmdOutput{
		changes: make([]fileDiff, 0),
	}

	/*
		The diff logic is implemented as follows:

		1) For each file in _after_ snapshot, check if it existed at the same path in the _before_ snapshot:
		   - if it existed, check if its properties match the _before_ ones
		     * if they don't, mark the file [modified]
		   - if it didn't, check if a file with a matching checksum existed at a different path:
		     * if found, mark the file [moved] and check if its properties match the _before_ ones
		     * if none found, mark the file [new]

		2) For each file in _before_ snapshot, check if it exists in the *after* snapshot:
		   - if it doesn't, mark the file [deleted]
	*/

	err = snapBefore.Read(func(byPathBefore, byCSBefore *bolt.Bucket) error {
		return snapAfter.Read(func(byPathAfter, byCSAfter *bolt.Bucket) error {
			// If either one of the before/after snapshots is shallow, diff in shallow mode.
			if snapBefore.Metadata().Shallow || snapAfter.Metadata().Shallow {
				shallow = true
			}

			err := byPathAfter.ForEach(func(path, data []byte) error {
				fileInfoAfter := snapshot.FileInfo{}
				if err := snapshot.Unmarshal(data, &fileInfoAfter); err != nil {
					return fmt.Errorf("unable to read snapshot data: %w", err)
				}

				// Skip files matching the excluded patterns.
				if excluded.Match(strings.Split(fileInfoAfter.Path, "/"), fileInfoAfter.IsDir) {
					return nil
				}

				if beforeData := byPathBefore.Get(path); beforeData != nil {
					// The file existed before, check if its properties have changed.
					fileInfoBefore := snapshot.FileInfo{}
					if err := snapshot.Unmarshal(beforeData, &fileInfoBefore); err != nil {
						return fmt.Errorf("unable to read snapshot data: %w", err)
					}

					changes := c.compareFiles(&fileInfoBefore, &fileInfoAfter)
					if len(changes) > 0 && !c.IgnoreModified {
						out.changes = append(out.changes, fileDiff{
							diffType:   diffTypeModified,
							fileBefore: &fileInfoBefore,
							fileAfter:  &fileInfoAfter,
							changes:    changes,
						})
						out.summary.modified++
					}
					return nil
				}

				// No file existed before at this path, check by checksum to see if it's a previous file moved
				// elsewhere -- unless we're in shallow mode, since we don't have the files' checksum.
				// We skip empty files, as they cause false positives by having identical checksum.
				if fileInfoAfter.Size > 0 && !shallow {
					if beforeData := byCSBefore.Get(fileInfoAfter.Checksum); beforeData != nil && !c.IgnoreModified {
						// The file existed before elsewhere, also check if its properties have changed.
						fileInfoBefore := snapshot.FileInfo{}
						if err := snapshot.Unmarshal(beforeData, &fileInfoBefore); err != nil {
							return fmt.Errorf("unable to read snapshot data: %w", err)
						}

						moved[fileInfoBefore.Path] = struct{}{}

						changes := c.compareFiles(&fileInfoBefore, &fileInfoAfter)
						out.changes = append(out.changes, fileDiff{
							diffType:   diffTypeModified,
							fileBefore: &fileInfoBefore,
							fileAfter:  &fileInfoAfter,
							changes:    changes,
						})
						out.summary.modified++
						return nil
					}
				}

				// No "before" file matches this checksum: this is a new file.
				if !c.IgnoreNew {
					out.changes = append(out.changes, fileDiff{
						diffType:  diffTypeNew,
						fileAfter: &fileInfoAfter,
					})
					out.summary.new++
				}
				return nil
			})
			if err != nil {
				return err
			}

			// Perform reverse lookup to detect deleted files.
			if err := byPathBefore.ForEach(func(path, data []byte) error {
				if afterData := byPathAfter.Get(path); afterData == nil {
					// Before marking a file as deleted, check if it is not the result of a renaming.
					if _, ok := moved[string(path)]; !ok {
						fileInfoBefore := snapshot.FileInfo{}
						if err := snapshot.Unmarshal(data, &fileInfoBefore); err != nil {
							return fmt.Errorf("unable to read snapshot data: %w", err)
						}
						if excluded.Match(strings.Split(fileInfoBefore.Path, "/"), fileInfoBefore.IsDir) {
							return nil
						}

						if !c.IgnoreDeleted {
							out.changes = append(out.changes, fileDiff{
								diffType:  diffTypeDeleted,
								fileAfter: &snapshot.FileInfo{Path: string(path)},
							})
							out.summary.deleted++
						}
					}
				}

				return nil
			}); err != nil {
				return fmt.Errorf("bolt: unable to loop on bucket keys: %w", err)
			}

			return nil
		})
	})
	if err != nil {
		return diffCmdOutput{}, err
	}

	return out, nil
}

func (c *diffCmd) compareFiles(before, after *snapshot.FileInfo) map[string][2]interface{} {
	diff := make(map[string][2]interface{})

	if !c.ignored("size") {
		if before.Size != after.Size {
			diff["size"] = [2]interface{}{before.Size, after.Size}
		}
	}

	if !c.ignored("mtime") {
		if !before.Mtime.Equal(after.Mtime) {
			diff["mtime"] = [2]interface{}{before.Mtime, after.Mtime}
		}
	}

	if !c.ignored("uid") {
		if before.Uid != after.Uid {
			diff["uid"] = [2]interface{}{before.Uid, after.Uid}
		}
	}

	if !c.ignored("gid") {
		if before.Gid != after.Gid {
			diff["gid"] = [2]interface{}{before.Gid, after.Gid}
		}
	}

	if !c.ignored("mode") {
		if before.Mode != after.Mode {
			diff["mode"] = [2]interface{}{before.Mode, after.Mode}
		}
	}

	if before.LinkTo != after.LinkTo {
		diff["link"] = [2]interface{}{before.LinkTo, after.LinkTo}
	}

	if before.IsDir != after.IsDir {
		diff["dir"] = [2]interface{}{before.IsDir, after.IsDir}
	}

	if before.IsSock != after.IsSock {
		diff["sock"] = [2]interface{}{before.IsSock, after.IsSock}
	}

	if before.IsPipe != after.IsPipe {
		diff["pipe"] = [2]interface{}{before.IsPipe, after.IsPipe}
	}

	if before.IsDev != after.IsDev {
		diff["dev"] = [2]interface{}{before.IsDev, after.IsDev}
	}

	if !c.ignored("checksum") && (before.Checksum != nil && after.Checksum != nil) {
		if !bytes.Equal(before.Checksum, after.Checksum) {
			diff["checksum"] = [2]interface{}{before.Checksum, after.Checksum}
		}
	}

	return diff
}

// ignored returns true if property p is in the ignored list, otherwise false.
func (c *diffCmd) ignored(p string) bool {
	for i := range c.Ignore {
		if c.Ignore[i] == p {
			return true
		}
	}

	return false
}

func (c *diffCmd) printNew(w io.Writer, f string) {
	_, _ = fmt.Fprintln(w, ansi.Color("+", "green"), f)
}

func (c *diffCmd) printModified(w io.Writer, before, after *snapshot.FileInfo, diff map[string][2]interface{}) {
	if before.Path != after.Path {
		_, _ = fmt.Fprintf(w, "%s %s => %s\n", ansi.Color(">", "cyan"), before.Path, after.Path)
	} else {
		_, _ = fmt.Fprintf(w, "%s %s\n", ansi.Color("~", "yellow"), after.Path)
	}

	if len(diff) > 0 {
		_, _ = fmt.Fprintf(w, "  %s\n  %s\n", before.String(), after.String())
	}
}

func (c *diffCmd) printDeleted(w io.Writer, f string) {
	_, _ = fmt.Fprintln(w, ansi.Color("-", "red"), f)
}

func (c *diffCmd) Run(ctx kong.Context) error {
	if c.NoColor {
		ansi.DisableColors(true)
	}

	out, err := c.run()
	if err != nil {
		ctx.Exit(2)
	}

	if !c.SummaryOnly {
		for _, fc := range out.changes {
			switch fc.diffType {
			case diffTypeNew:
				c.printNew(ctx.Stdout, fc.fileAfter.Path)
			case diffTypeModified:
				c.printModified(ctx.Stdout, fc.fileBefore, fc.fileAfter, fc.changes)
			case diffTypeDeleted:
				c.printDeleted(ctx.Stdout, fc.fileAfter.Path)
			}
		}
		_, _ = fmt.Fprintln(ctx.Stdout)
	}

	if out.summary.new > 0 || out.summary.modified > 0 || out.summary.deleted > 0 {
		if !c.Quiet {
			_, _ = fmt.Fprintf(
				ctx.Stdout,
				"%d new, %d modified, %d deleted\n",
				out.summary.new,
				out.summary.modified,
				out.summary.deleted,
			)
		}
		ctx.Exit(1)
	}

	return nil
}
