package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/falzm/fsdiff/snapshot"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	cmdSnapshot = kingpin.Command("snapshot", "Scan file tree and record object properties").
			Alias("snap")
	cmdSnapshotFlagOut = cmdSnapshot.Flag("output-file",
		"File path to write snapshot to (default: <YYYYMMDDhhmmss>.snap)").
		Short('o').
		String()
	cmdSnapshotFlagCarryOn = cmdSnapshot.Flag("carry-on", "Continue on filesystem error").
				Bool()
	cmdSnapshotFlagShallow = cmdSnapshot.Flag("shallow", "Don't calculate files checksum").
				Bool()
	cmdSnapshotArgRoot = cmdSnapshot.Arg("root", "Path to root directory").
				Required().
				ExistingDir()
)

func doSnapshot(root, out string, carryOn bool, shallow bool) error {
	if out == "" {
		out = time.Now().Format("20060102150405.snap")
	}

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
