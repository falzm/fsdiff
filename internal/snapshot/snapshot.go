// Package snapshot manages file tree snapshots created by the "fsdiff snapshot" command.
package snapshot

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	bolt "go.etcd.io/bbolt"
	"gopkg.in/src-d/go-git.v4/plumbing/format/gitignore"

	"github.com/falzm/fsdiff/internal/version"
)

const (
	byChecksumBucket = "by_cs"
	byPathBucket     = "by_path"
	metadataBucket   = "metadata"
)

// FormatVersion represents the current snapshot file format version.
const FormatVersion = 1

// Metadata represent a Snapshot metadata.
type Metadata struct {
	// FormatVersion is the snapshot format version, for backward compatibility.
	FormatVersion int

	// FsdiffVersion is the version of the "fsdiff" command that has been used to create the snapshot.
	FsdiffVersion string

	// Date is the date when the snapshot has been created.
	Date time.Time

	// RootDir is the absolute path to the snapshotted root directory.
	RootDir string

	// Shallow indicates if the snapshot has been done in "shallow" mode.
	Shallow bool
}

// Snapshot represents a filesystem snapshot.
type Snapshot struct {
	db   *bolt.DB
	meta Metadata
}

type createSnapshotOptions struct {
	carryOn  bool
	shallow  bool
	excluded gitignore.Matcher
}

// CreateOpt represents a Snapshot creation option.
type CreateOpt func(c *createSnapshotOptions)

// CreateOptCarryOn sets the Snapshot creation to continue in case of filesystem error.
func CreateOptCarryOn() CreateOpt {
	return func(o *createSnapshotOptions) {
		o.carryOn = true
	}
}

// CreateOptExclude sets at list of gitignore-compatible exclusion pattern.
func CreateOptExclude(v []string) CreateOpt {
	return func(o *createSnapshotOptions) {
		patterns := make([]gitignore.Pattern, len(v))
		for i, p := range v {
			patterns[i] = gitignore.ParsePattern(p, nil)
		}
		o.excluded = gitignore.NewMatcher(patterns)
	}
}

// CreateOptShallow sets the Snapshot creation to skip files checksum computation.
func CreateOptShallow() CreateOpt {
	return func(o *createSnapshotOptions) {
		o.shallow = true
	}
}

// newSnapshot creates a new empty snapshot file stored at <outFile> and initializes its metadata.
func newSnapshot(outFile, root string, shallow bool) (*Snapshot, error) {
	var snap Snapshot

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("unable to get root directory absolute path: %w", err)
	}

	if snap.db, err = bolt.Open(outFile, 0o600, &bolt.Options{
		Timeout: 1 * time.Second,
		OpenFile: func(name string, flag int, perm os.FileMode) (*os.File, error) {
			f, err := os.OpenFile(name, flag, perm)
			if err != nil {
				return nil, err
			}

			return f, f.Truncate(0)
		},
	}); err != nil {
		return nil, err
	}

	snap.meta = Metadata{
		FormatVersion: FormatVersion,
		FsdiffVersion: version.Version + " " + version.Commit,
		Date:          time.Now(),
		RootDir:       absRoot,
		Shallow:       shallow,
	}

	if err = snap.db.Update(func(tx *bolt.Tx) error {
		var mdBucket *bolt.Bucket

		if _, err = tx.CreateBucket([]byte(byChecksumBucket)); err != nil {
			return fmt.Errorf("bolt: unable to create bucket %q: %w", byChecksumBucket, err)
		}

		if _, err = tx.CreateBucket([]byte(byPathBucket)); err != nil {
			return fmt.Errorf("bolt: unable to create bucket %q: %w", byPathBucket, err)
		}

		if mdBucket, err = tx.CreateBucket([]byte(metadataBucket)); err != nil {
			return fmt.Errorf("bolt: unable to create bucket %q: %w", metadataBucket, err)
		}

		snapshotInfo, err := Marshal(snap.meta)
		if err != nil {
			return err
		}

		if err := mdBucket.Put([]byte("info"), snapshotInfo); err != nil {
			return fmt.Errorf("bolt: unable to write metadata: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &snap, nil
}

// Create creates a new Snapshot of directory <root> to be stored to file <outFile>. If the <shallow> argument is
// true, the snapshot will be performed in "shallow" mode (i.e. without computing files checksum).
func Create(outFile, root string, opts ...CreateOpt) (*Snapshot, error) {
	options := createSnapshotOptions{
		excluded: gitignore.NewMatcher(nil),
	}
	for _, o := range opts {
		o(&options)
	}

	if !strings.HasSuffix(root, "/") {
		root += "/"
	}

	snap, err := newSnapshot(outFile, root, options.shallow)
	if err != nil {
		return nil, err
	}

	err = snap.Write(func(byPath, byCS *bolt.Bucket) error {
		return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			// Skip the root directory itself
			if path == root {
				return nil
			}

			// Skip files matching the excluded patterns
			if options.excluded.Match(strings.Split(strings.TrimPrefix(path, root), "/"), info.IsDir()) {
				return nil
			}

			if err != nil {
				if options.carryOn {
					return nil
				}
				return err
			}

			f := FileInfo{
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
					if options.carryOn {
						return nil
					}
					return fmt.Errorf("unable to read symlink: %w", err)
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
			if !options.shallow && !f.IsDir && !f.IsSock && !f.IsPipe && !f.IsDev && f.LinkTo == "" {
				if f.Checksum, err = checksumFile(path); err != nil {
					if options.carryOn {
						return nil
					}
					return fmt.Errorf("unable to compute file checksum: %w", err)
				}

				data, err := Marshal(f)
				if err != nil {
					return fmt.Errorf("unable to serialize snapshot data: %w", err)
				}
				if err := byCS.Put(f.Checksum, data); err != nil {
					return fmt.Errorf("bolt: unable to write to bucket: %w", err)
				}
			}

			data, err := Marshal(f)
			if err != nil {
				return fmt.Errorf("unable to serialize snapshot data: %w", err)
			}
			if err := byPath.Put([]byte(strings.TrimPrefix(path, root)), data); err != nil {
				return fmt.Errorf("bolt: unable to write to bucket: %w", err)
			}

			return nil
		})
	})

	return snap, err
}

// Open opens the Snapshot file at <path> in read-only mode.
func Open(path string) (*Snapshot, error) {
	var (
		snap Snapshot
		err  error
	)

	if snap.db, err = bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
		return nil, err
	}

	if err = snap.db.View(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket([]byte(metadataBucket))
		if metaBucket == nil {
			return errors.New(`"metadata" bucket not found in snapshot file`)
		}

		data := metaBucket.Get([]byte("info"))
		if data == nil {
			return errors.New("invalid snapshot metadata")
		}

		if err := Unmarshal(data, &snap.meta); err != nil {
			return fmt.Errorf("unable to read metadata: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &snap, nil
}

// Write executes the <writeFunc> function in a read-write transaction of the Snapshot database.
func (s *Snapshot) Write(writeFunc func(byPath, byChecksum *bolt.Bucket) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		var (
			pathBucket *bolt.Bucket
			csBucket   *bolt.Bucket
		)

		if pathBucket = tx.Bucket([]byte(byPathBucket)); pathBucket == nil {
			return fmt.Errorf("bolt: unable to retrieve bucket %q", byPathBucket)
		}

		if csBucket = tx.Bucket([]byte(byChecksumBucket)); csBucket == nil {
			return fmt.Errorf("bolt: unable to retrieve bucket %q", byChecksumBucket)
		}

		return writeFunc(pathBucket, csBucket)
	})
}

// Read executes the <readFunc> function in a read-only transaction of the Snapshot database.
func (s *Snapshot) Read(readFunc func(byPath, byChecksum *bolt.Bucket) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		var (
			pathBucket *bolt.Bucket
			csBucket   *bolt.Bucket
		)

		if pathBucket = tx.Bucket([]byte(byPathBucket)); pathBucket == nil {
			return fmt.Errorf("bolt: unable to retrieve %q bucket", byPathBucket)
		}

		if csBucket = tx.Bucket([]byte(byChecksumBucket)); csBucket == nil {
			return fmt.Errorf("bolt: unable to retrieve %q bucket", byChecksumBucket)
		}

		return readFunc(pathBucket, csBucket)
	})
}

// FilesByChecksum returns a list of FileInfo referenced by checksum in the Snapshot.
func (s *Snapshot) FilesByChecksum() ([]*FileInfo, error) {
	files := make([]*FileInfo, 0)

	err := s.Read(func(_, byChecksum *bolt.Bucket) error {
		c := byChecksum.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			fi := FileInfo{}
			if err := Unmarshal(v, &fi); err != nil {
				return fmt.Errorf("unable to unmarshal file information data: %w", err)
			}
			files = append(files, &fi)
		}

		return nil
	})

	return files, err
}

// FilesByPath returns a list of FileInfo referenced by path in the Snapshot.
func (s *Snapshot) FilesByPath() ([]*FileInfo, error) {
	files := make([]*FileInfo, 0)

	err := s.Read(func(byPath, _ *bolt.Bucket) error {
		c := byPath.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			fi := FileInfo{}
			if err := Unmarshal(v, &fi); err != nil {
				return fmt.Errorf("unable to unmarshal file information data: %w", err)
			}
			files = append(files, &fi)
		}

		return nil
	})

	return files, err
}

// Metadata returns the Snapshot metadata.
func (s *Snapshot) Metadata() *Metadata {
	return &s.meta
}

// Close closes the Snapshot database session.
func (s *Snapshot) Close() error {
	return s.db.Close()
}

// Marshal serializes <v> in raw data for Storage in the snapshot database.
func Marshal(v interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)

	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("gob: cannot marshal: %w", err)
	}

	return buf.Bytes(), nil
}

// Unmarshal deserializes Snapshot database <data> into <v>.
func Unmarshal(data []byte, v interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("gob: cannot unmarshal: %w", err)
	}

	return nil
}
