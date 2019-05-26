// Package snapshot manages snapshot files.
package snapshot

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

const SNAPSHOT_VERSION = 1

var (
	version string
	commit  string
)

// Metadata represent a Snapshot metadata.
type Metadata struct {
	// FormatVersion is the snapshot format version, for backward
	// compatibility.
	FormatVersion int

	// FsdiffVersion is the version of the "fsdiff" command that has been used
	// to create the snapshot.
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
	db      *bolt.DB
	meta    Metadata
	root    string
	shallow bool
}

// New creates a new snapshot of directory <root> to file <out>. If the
// <shallow> argument is true, the snapshot will be performed in
// "shallow" mode (i.e. without computing files checksum).
func New(out, root string, shallow bool) (*Snapshot, error) {
	var (
		snap Snapshot
		err  error
	)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get root directory absolute path")
	}

	if snap.db, err = bolt.Open(out, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
		OpenFile: func(name string, flag int, perm os.FileMode) (*os.File, error) {
			f, err := os.OpenFile(name, flag, perm)
			if err != nil {
				return nil, err
			}
			f.Truncate(0)
			return f, nil
		},
	}); err != nil {
		return nil, err
	}

	snap.meta = Metadata{
		FormatVersion: SNAPSHOT_VERSION,
		FsdiffVersion: version + " " + commit,
		Date:          time.Now(),
		RootDir:       absRoot,
		Shallow:       shallow,
	}

	if err = snap.db.Update(func(tx *bolt.Tx) error {
		var metaBucket *bolt.Bucket

		if metaBucket, err = tx.CreateBucket([]byte("metadata")); err != nil {
			return errors.Wrap(err, "bolt: unable to create bucket")
		}

		data, err := Marshal(snap.meta)
		if err != nil {
			return err
		}

		if err := metaBucket.Put([]byte("info"), data); err != nil {
			return errors.Wrap(err, "bolt: unable to write metadata")
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &snap, nil
}

// Open opens the snapshot file at <path> in read-only mode.
func Open(path string) (*Snapshot, error) {
	var (
		snap Snapshot
		err  error
	)

	if snap.db, err = bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second}); err != nil {
		return nil, err
	}

	if err = snap.db.View(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket([]byte("metadata"))
		if metaBucket == nil {
			return errors.New(`"metadata" bucket not found in snapshot file`)
		}

		data := metaBucket.Get([]byte("info"))
		if data == nil {
			return errors.New("invalid snapshot metadata")
		}

		if err := Unmarshal(data, &snap.meta); err != nil {
			return errors.Wrap(err, "unable to read metadata")
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &snap, nil
}

// Write executes the <writeFunc> function in a read-write tramsaction of the
// snapshot database.
func (s *Snapshot) Write(writeFunc func(byPath, byCS *bolt.Bucket) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		var (
			pathBucket *bolt.Bucket
			csBucket   *bolt.Bucket
			err        error
		)

		if pathBucket, err = tx.CreateBucket([]byte("by_path")); err != nil {
			return errors.Wrap(err, "bolt: unable to create bucket")
		}

		if csBucket, err = tx.CreateBucket([]byte("by_cs")); err != nil {
			return errors.Wrap(err, "bolt: unable to create bucket")
		}

		return writeFunc(pathBucket, csBucket)
	})
}

// Read executes the <readFunc> function in a read-only transaction of the
// snapshot database.
func (s *Snapshot) Read(readFunc func(byPath, byCS *bolt.Bucket) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		var (
			pathBucket *bolt.Bucket
			csBucket   *bolt.Bucket
		)

		if pathBucket = tx.Bucket([]byte("by_path")); pathBucket == nil {
			return errors.New(`"by_path" bucket not found in snapshot file`)
		}

		if csBucket = tx.Bucket([]byte("by_cs")); csBucket == nil {
			return errors.New(`"by_cs" bucket not found in snapshot file`)
		}

		return readFunc(pathBucket, csBucket)
	})
}

// Metadata returns the snapshot metadata.
func (s *Snapshot) Metadata() *Metadata {
	return &s.meta
}

// Close closes the snapshot database session.
func (s *Snapshot) Close() error {
	return s.db.Close()
}

// Marshal serializes <v> in raw data for storage in the snapshot database.
func Marshal(v interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)

	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("gob: cannot marshal: %s", err)
	}

	return buf.Bytes(), nil
}

// Marshal unserializes snapshot database <data> into <v>.
func Unmarshal(data []byte, v interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("gob: cannot unmarshal: %s", err)
	}

	return nil
}
