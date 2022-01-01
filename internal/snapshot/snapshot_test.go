package snapshot

import (
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"

	"github.com/falzm/fsdiff/internal/version"
)

var testSeededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

type testSuite struct {
	suite.Suite

	testDir string
	rootDir string
}

func (ts *testSuite) SetupTest() {
	dir, err := os.MkdirTemp(os.TempDir(), "fsdiff-test-*")
	ts.Require().NoError(err)
	ts.testDir = dir

	ts.rootDir = filepath.Join(ts.testDir, "root")
	ts.Require().NoError(os.Mkdir(ts.rootDir, 0o755))
}

func (ts *testSuite) TearDownTest() {
	ts.Require().NoError(os.RemoveAll(ts.testDir))
}

func (ts *testSuite) randomStringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[testSeededRand.Intn(len(charset))]
	}
	return string(b)
}

func (ts *testSuite) randomString(length int) string {
	const defaultCharset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	return ts.randomStringWithCharset(length, defaultCharset)
}

func (ts *testSuite) createDummyFile(path string, data []byte, mode os.FileMode) string {
	ts.Require().NoError(os.MkdirAll(filepath.Join(ts.rootDir, filepath.Dir(path)), 0o755))
	path = filepath.Join(ts.rootDir, path)

	ts.Require().NoError(os.WriteFile(path, data, mode))

	return path
}

func (ts *testSuite) TestNewSnapshot() {
	actual, err := newSnapshot(path.Join(ts.rootDir, "test.snap"), ts.rootDir, true)
	ts.Require().NoError(err)
	_ = actual.db.View(func(tx *bolt.Tx) error {
		ts.Require().NotNil(tx.Bucket([]byte(byPathBucket)))
		ts.Require().NotNil(tx.Bucket([]byte(byChecksumBucket)))
		ts.Require().NotNil(tx.Bucket([]byte(metadataBucket)))
		return nil
	})
	ts.Require().FileExists(path.Join(ts.rootDir, "test.snap"))
	ts.Require().Equal(FormatVersion, actual.meta.FormatVersion)
	ts.Require().Equal(version.Version+" "+version.Commit, actual.meta.FsdiffVersion)
	ts.Require().True(actual.meta.Date.After(time.Now().Add(-time.Minute)))
	ts.Require().Equal(ts.rootDir, actual.meta.RootDir)
	ts.Require().True(actual.meta.Shallow)
}

func (ts *testSuite) TestCreate() {
	tests := []struct {
		name      string
		opts      []CreateOpt
		setupFunc func(*testSuite)
		testFunc  func(*testSuite, *Snapshot, error)
	}{
		{
			name:      "full",
			setupFunc: func(t *testSuite) { ts.createDummyFile("x", []byte("x"), 0o644) },
			testFunc: func(ts *testSuite, actual *Snapshot, err error) {
				ts.Require().NoError(err)
				ts.Require().NotNil(actual)
				defer actual.Close()

				// Check that the snapshot references only our test file "x".
				ts.Require().NoError(actual.Read(func(byPath, byCS *bolt.Bucket) error {
					var (
						data         []byte
						testFileInfo FileInfo
					)

					// By path:
					ts.Require().Equal(1, byPath.Stats().KeyN)
					data = byPath.Get([]byte("x"))
					ts.Require().NotNil(data)
					ts.Require().NoError(Unmarshal(data, &testFileInfo))
					ts.Require().Equal("x", testFileInfo.Path)
					ts.Require().NotEmpty(testFileInfo.Checksum)
					ts.Require().NotEmpty(testFileInfo.Gid)
					ts.Require().NotEmpty(testFileInfo.Mode)
					ts.Require().NotEmpty(testFileInfo.Mtime)
					ts.Require().NotEmpty(testFileInfo.Size)
					ts.Require().NotEmpty(testFileInfo.Uid)

					// By checksum:
					testFileChecksum, err := checksumFile(filepath.Join(ts.rootDir, "x"))
					ts.Require().NoError(err)
					ts.Require().Equal(1, byCS.Stats().KeyN)
					data = byCS.Get(testFileChecksum)
					ts.Require().NotNil(data)
					ts.Require().NoError(Unmarshal(data, &testFileInfo))
					ts.Require().Equal("x", testFileInfo.Path)

					return nil
				}))
			},
		},
		{
			name:      "shallow",
			opts:      []CreateOpt{CreateOptShallow()},
			setupFunc: func(t *testSuite) { ts.createDummyFile("x", []byte("x"), 0o644) },
			testFunc: func(ts *testSuite, actual *Snapshot, err error) {
				ts.Require().NoError(err)
				ts.Require().NotNil(actual)
				defer actual.Close()

				// Check that the snapshot references only our test file "x".
				ts.Require().NoError(actual.Read(func(byPath, byCS *bolt.Bucket) error {
					var (
						data         []byte
						testFileInfo FileInfo
					)

					// By path:
					ts.Require().Equal(1, byPath.Stats().KeyN)
					data = byPath.Get([]byte("x"))
					ts.Require().NotNil(data)
					ts.Require().NoError(Unmarshal(data, &testFileInfo))
					ts.Require().Equal("x", testFileInfo.Path)
					ts.Require().Empty(testFileInfo.Checksum)

					// By checksum:
					ts.Require().Equal(0, byCS.Stats().KeyN)

					return nil
				}))
			},
		},
		{
			name: "with excludes",
			opts: []CreateOpt{CreateOptExclude([]string{"b"})},
			setupFunc: func(t *testSuite) {
				ts.createDummyFile("a", []byte("a"), 0o644)
				ts.createDummyFile("b", []byte("b"), 0o644)
			},
			testFunc: func(ts *testSuite, actual *Snapshot, err error) {
				ts.Require().NoError(err)
				ts.Require().NotNil(actual)
				defer actual.Close()

				// Check that the snapshot references only our test file "a".
				ts.Require().NoError(actual.Read(func(byPath, byCS *bolt.Bucket) error {
					ts.Require().Equal(1, byPath.Stats().KeyN)
					ts.Require().NotNil(byPath.Get([]byte("a")))

					return nil
				}))
			},
		},
		{
			name:      "filesystem error without carry-on",
			setupFunc: func(t *testSuite) { ts.createDummyFile("x", []byte("x"), 0o000) },
			testFunc:  func(ts *testSuite, actual *Snapshot, err error) { ts.Require().Error(err) },
		},
		{
			name:      "filesystem error with carry-on",
			opts:      []CreateOpt{CreateOptCarryOn()},
			setupFunc: func(t *testSuite) { ts.createDummyFile("x", []byte("x"), 0o000) },
			testFunc: func(ts *testSuite, actual *Snapshot, err error) {
				ts.Require().NoError(err)
				ts.Require().NotNil(actual)
				defer actual.Close()

				// Check that the snapshot references only our test file "x".
				ts.Require().NoError(actual.Read(func(byPath, byCS *bolt.Bucket) error {
					// By path:
					ts.Require().Equal(0, byPath.Stats().KeyN)

					// By checksum:
					ts.Require().Equal(0, byCS.Stats().KeyN)

					return nil
				}))
			},
		},
	}

	for _, tt := range tests {
		ts.T().Run(tt.name, func(t *testing.T) {
			// Clean up root dir between test cases.
			dir, err := os.ReadDir(ts.rootDir)
			ts.Require().NoError(err)
			for _, de := range dir {
				ts.Require().NoError(os.RemoveAll(path.Join(ts.rootDir, de.Name())))
			}

			if tt.setupFunc != nil {
				tt.setupFunc(ts)
			}

			actual, err := Create(
				path.Join(ts.testDir, ts.randomString(10)+".snap"),
				ts.rootDir,
				tt.opts...,
			)
			tt.testFunc(ts, actual, err)
		})
	}
}

func (ts *testSuite) TestOpen() {
	snap, err := newSnapshot(path.Join(ts.testDir, "test.snap"), ts.rootDir, true)
	ts.Require().NoError(err)
	ts.Require().NoError(snap.Close())

	actual, err := Open(path.Join(ts.testDir, "test.snap"))
	ts.Require().NoError(err)
	ts.Require().NotNil(actual)
	ts.Require().NoError(actual.Close())
}

func (ts *testSuite) TestCreateOptions() {
	var actual createSnapshotOptions

	for _, o := range []CreateOpt{
		CreateOptCarryOn(),
		CreateOptExclude([]string{"test"}),
		CreateOptShallow(),
	} {
		o(&actual)
	}

	ts.Require().True(actual.carryOn)
	ts.Require().NotNil(actual.excluded)
	ts.Require().True(actual.shallow)
}

func (ts *testSuite) TestSnapshot_Write() {
	snap, err := newSnapshot(path.Join(ts.testDir, "test.snap"), ts.rootDir, true)
	ts.Require().NoError(err)
	ts.Require().NoError(snap.Write(func(byPath, byChecksum *bolt.Bucket) error {
		ts.Require().NoError(byPath.Put([]byte("path1"), []byte("foo")))
		ts.Require().NoError(byChecksum.Put([]byte("cs1"), []byte("bar")))
		return nil
	}))
	_ = snap.db.View(func(tx *bolt.Tx) error {
		ts.Require().Equal([]byte("foo"), tx.Bucket([]byte(byPathBucket)).Get([]byte("path1")))
		ts.Require().Equal([]byte("bar"), tx.Bucket([]byte(byChecksumBucket)).Get([]byte("cs1")))
		return nil
	})
	ts.Require().NoError(snap.Close())
}

func (ts *testSuite) TestSnapshot_Read() {
	snap, err := newSnapshot(path.Join(ts.testDir, "test.snap"), ts.rootDir, true)
	ts.Require().NoError(err)
	_ = snap.db.Update(func(tx *bolt.Tx) error {
		ts.Require().NoError(tx.Bucket([]byte(byPathBucket)).Put([]byte("path1"), []byte("foo")))
		ts.Require().NoError(tx.Bucket([]byte(byChecksumBucket)).Put([]byte("cs1"), []byte("bar")))
		return nil
	})
	ts.Require().NoError(snap.Read(func(byPath, byChecksum *bolt.Bucket) error {
		ts.Require().Equal([]byte("foo"), byPath.Get([]byte("path1")))
		ts.Require().Equal([]byte("bar"), byChecksum.Get([]byte("cs1")))
		return nil
	}))
	ts.Require().NoError(snap.Close())
}

func TestSnapshotTestSuite(t *testing.T) {
	suite.Run(t, new(testSuite))
}
