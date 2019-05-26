package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/falzm/fsdiff/snapshot"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/src-d/go-git.v4/plumbing/format/gitignore"
)

var (
	cmdSnapshot = kingpin.Command("snapshot", "Scan file tree and record object properties").
			Alias("snap")
	cmdSnapshotFlagCarryOn = cmdSnapshot.Flag("carry-on", "Continue on filesystem error").
				Bool()
	cmdSnapshotFlagExclude = cmdSnapshot.Flag("exclude",
		"gitignore-compatible exclusion pattern (see https://git-scm.com/docs/gitignore)").
		Strings()
	cmdSnapshotFlagExcludeFrom = cmdSnapshot.Flag("exclude-from",
		"File path to read gitignore-compatible patterns from (see https://git-scm.com/docs/gitignore)").
		ExistingFile()
	cmdSnapshotFlagOut = cmdSnapshot.Flag("output-file",
		"File path to write snapshot to (default: <YYYYMMDDhhmmss>.snap)").
		Short('o').
		String()
	cmdSnapshotFlagShallow = cmdSnapshot.Flag("shallow", "Don't calculate files checksum").
				Bool()
	cmdSnapshotArgRoot = cmdSnapshot.Arg("root", "Path to root directory").
				Required().
				ExistingDir()
)

func doSnapshot() error {
	var (
		root        = *cmdSnapshotArgRoot
		carryOn     = *cmdSnapshotFlagCarryOn
		out         = *cmdSnapshotFlagOut
		shallow     = *cmdSnapshotFlagShallow
		exclude     = *cmdSnapshotFlagExclude
		excludeFrom = *cmdSnapshotFlagExcludeFrom

		excludedPatterns []gitignore.Pattern
		excluded         gitignore.Matcher
		err              error
	)

	if out == "" {
		out = time.Now().Format("20060102150405.snap")
	}

	if excludeFrom != "" {
		if excludedPatterns, err = loadExcludeFile(excludeFrom); err != nil {
			return errors.Wrap(err, "unable to load ignore file")
		}
	}
	for _, p := range exclude {
		excludedPatterns = append(excludedPatterns, gitignore.ParsePattern(p, nil))
	}
	excluded = gitignore.NewMatcher(excludedPatterns)

	if !strings.HasSuffix(root, "/") {
		root += "/"
	}

	snap, err := snapshot.New(out, root, shallow)
	if err != nil {
		return errors.Wrap(err, "unable to open snapshot file")
	}
	defer snap.Close()

	return snap.Write(func(byPath, byCS *bolt.Bucket) error {
		if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			// Skip the root directory itself
			if path == root {
				return nil
			}

			// Skip files matching the excluded patterns
			if excluded.Match(strings.Split(strings.TrimPrefix(path, root), "/"), info.IsDir()) {
				return nil
			}

			if err != nil {
				if carryOn {
					return nil
				}
				return err
			}

			f := fileInfo{
				Size:  info.Size(),
				Mtime: info.ModTime(),
				Uid:   info.Sys().(*syscall.Stat_t).Uid,
				Gid:   info.Sys().(*syscall.Stat_t).Gid,
				Mode:  info.Mode(),
				IsDir: info.IsDir(),
				Path:  strings.TrimPrefix(path, root),
			}

			if f.Mode&os.ModeSymlink == os.ModeSymlink {
				f.LinkTo, err = os.Readlink(path)
				if err != nil {
					if carryOn {
						return nil
					}
					return errors.Wrap(err, "unable to read symlink")
				}
			}

			if f.Mode&os.ModeSocket == os.ModeSocket {
				f.IsSock = true
			} else if f.Mode&os.ModeNamedPipe == os.ModeNamedPipe {
				f.IsPipe = true
			} else if f.Mode&os.ModeDevice == os.ModeDevice || f.Mode&os.ModeCharDevice == os.ModeCharDevice {
				f.IsDev = true
			}

			// Index regular files also by checksum for reverse lookup during diff unless running in "shallow" mode
			if !shallow && !f.IsDir && !f.IsSock && !f.IsPipe && !f.IsDev && f.LinkTo == "" {
				if f.Checksum, err = checksumFile(path); err != nil {
					return errors.Wrap(err, "unable to compute file checksum")
				}

				data, err := snapshot.Marshal(f)
				if err != nil {
					return errors.Wrap(err, "unable to serialize snapshot data")
				}
				if err := byCS.Put(f.Checksum, data); err != nil {
					return errors.Wrap(err, "bolt: unable to write to bucket")
				}
			}

			data, err := snapshot.Marshal(f)
			if err != nil {
				return errors.Wrap(err, "unable to serialize snapshot data")
			}
			if err := byPath.Put([]byte(strings.TrimPrefix(path, root)), data); err != nil {
				return errors.Wrap(err, "bolt: unable to write to bucket")
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	})
}

func loadExcludeFile(path string) ([]gitignore.Pattern, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	patterns := make([]gitignore.Pattern, 0)
	for _, s := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(s, "#") && len(strings.TrimSpace(s)) > 0 {
			patterns = append(patterns, gitignore.ParsePattern(s, nil))
		}
	}

	return patterns, nil
}
