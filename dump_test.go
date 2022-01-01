package main

import (
	"path"

	"github.com/falzm/fsdiff/internal/snapshot"
)

func (ts *testSuite) TestDumpCmd_run() {
	ts.createDummyFile("x", []byte("x"), 0o644)

	snap, err := snapshot.Create(path.Join(ts.testDir, "test.snap"), ts.rootDir)
	ts.Require().NoError(err)
	ts.Require().NoError(snap.Close())

	cmd := dumpCmd{
		SnapshotFile: path.Join(ts.testDir, "test.snap"),
	}

	out, err := cmd.run()
	ts.Require().NoError(err)
	ts.Require().Len(out.filesByChecksum, 1)
	ts.Require().Len(out.filesByPath, 1)
	ts.Require().NotNil(out.metadata)
}
