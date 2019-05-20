package main

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

const SNAPSHOT_VERSION = 1

var (
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

type metadata struct {
	FormatVersion int
	FsdiffVersion string
	Date          time.Time
	RootDir       string
	Shallow       bool
}

func marshal(v interface{}) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)

	err := enc.Encode(v)
	if err != nil {
		dieOnError("gob: cannot marshal: %s", err)
	}

	return buf.Bytes()
}

func unmarshal(data []byte, v interface{}) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(v)
	if err != nil {
		dieOnError("gob: cannot unmarshal: %s", err)
	}
}

func snapshot(root, out string, carryOn bool, shallow bool) error {
	if out == "" {
		out = time.Now().Format("20060102150405.snap")
	}

	if !strings.HasSuffix(root, "/") {
		root += "/"
	}

	db, err := bolt.Open(out, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
		OpenFile: func(name string, flag int, perm os.FileMode) (*os.File, error) {
			f, err := os.OpenFile(name, flag, perm)
			if err != nil {
				return nil, err
			}
			f.Truncate(0)
			return f, nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "unable to open snapshot file")
	}
	defer db.Close()

	return db.Update(func(tx *bolt.Tx) error {
		var (
			metaBucket *bolt.Bucket
			pathBucket *bolt.Bucket
			csBucket   *bolt.Bucket
			err        error
		)

		if metaBucket, err = tx.CreateBucket([]byte("metadata")); err != nil {
			return errors.Wrap(err, "bolt: unable to create bucket")
		}

		absRoot, err := filepath.Abs(root)
		if err != nil {
			return errors.Wrap(err, "unable to get root directory absolute path")
		}

		if err := metaBucket.Put([]byte("info"), marshal(metadata{
			FormatVersion: SNAPSHOT_VERSION,
			FsdiffVersion: version + " " + commit,
			Date:          time.Now(),
			RootDir:       absRoot,
			Shallow:       shallow,
		})); err != nil {
			return errors.Wrap(err, "bolt: unable to write metadata")
		}

		if pathBucket, err = tx.CreateBucket([]byte("by_path")); err != nil {
			return errors.Wrap(err, "bolt: unable to create bucket")
		}

		if csBucket, err = tx.CreateBucket([]byte("by_cs")); err != nil {
			return errors.Wrap(err, "bolt: unable to create bucket")
		}

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

			// Index regular files (no directory or symlink) also by checksum for reverse lookup during diff
			// unless running in "shallow" mode
			if !shallow && !f.IsDir && f.LinkTo == "" {
				if f.Checksum, err = checksumFile(path); err != nil {
					dieOnError("unable to compute file checksum: %s", err)
				}

				if err := csBucket.Put(f.Checksum, marshal(f)); err != nil {
					return errors.Wrap(err, "bolt: unable to write to bucket")
				}
			}

			if err := pathBucket.Put([]byte(strings.TrimPrefix(path, root)), marshal(f)); err != nil {
				return errors.Wrap(err, "bolt: unable to write to bucket")
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	})
}
