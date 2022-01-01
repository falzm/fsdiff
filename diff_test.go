package main

import (
	"os"
	"path"
	"testing"

	"github.com/falzm/fsdiff/internal/snapshot"
)

func (ts *testSuite) TestDiffCmd_run() {
	ts.createDummyFile("a", []byte("a"), 0o644)
	ts.createDummyFile("b", []byte("b"), 0o644)
	ts.createDummyFile("c", []byte("c"), 0o644)

	snapBefore, err := snapshot.Create(path.Join(ts.testDir, "before.snap"), ts.rootDir)
	ts.Require().NoError(err)
	ts.Require().NoError(snapBefore.Close())

	ts.Require().NoError(os.Remove(path.Join(ts.rootDir, "b")))
	ts.Require().NoError(os.Chmod(path.Join(ts.rootDir, "a"), 0o640))
	ts.createDummyFile("x", []byte("x"), 0o644)
	ts.createDummyFile("c", []byte("cc"), 0o644)

	snapAfter, err := snapshot.Create(path.Join(ts.testDir, "after.snap"), ts.rootDir)
	ts.Require().NoError(err)
	ts.Require().NoError(snapAfter.Close())

	tests := []struct {
		name     string
		cmd      *diffCmd
		testFunc func(*testSuite, *diffCmdOutput)
	}{
		{
			name: "full",
			cmd: &diffCmd{
				Before: path.Join(ts.testDir, "before.snap"),
				After:  path.Join(ts.testDir, "after.snap"),
			},
			testFunc: func(ts *testSuite, out *diffCmdOutput) {
				ts.Require().Equal(1, out.summary.new)
				ts.Require().Equal(1, out.summary.deleted)
				ts.Require().Equal(2, out.summary.modified)
				ts.Require().Len(out.changes, 4)

				ts.Require().Equal("x", func() fileDiff {
					for _, d := range out.changes {
						if d.diffType == diffTypeNew {
							return d
						}
					}
					ts.T().Fatal("no new files found in changes list")
					return fileDiff{}
				}().fileAfter.Path)

				ts.Require().Equal("b", func() fileDiff {
					for _, d := range out.changes {
						if d.diffType == diffTypeDeleted {
							return d
						}
					}
					ts.T().Fatal("no deleted files found in changes list")
					return fileDiff{}
				}().fileAfter.Path)
			},
		},
		{
			name: "with --ignore",
			cmd: &diffCmd{
				Before: path.Join(ts.testDir, "before.snap"),
				After:  path.Join(ts.testDir, "after.snap"),
				Ignore: []string{"mode"},
			},
			testFunc: func(ts *testSuite, out *diffCmdOutput) {
				ts.Require().Equal(1, out.summary.new)
				ts.Require().Equal(1, out.summary.deleted)
				ts.Require().Equal(1, out.summary.modified)
				ts.Require().Len(out.changes, 3)

				ts.Require().Equal("c", func() fileDiff {
					for _, d := range out.changes {
						if d.diffType == diffTypeModified {
							return d
						}
					}
					ts.T().Fatal("no modified files found in changes list")
					return fileDiff{}
				}().fileAfter.Path)
			},
		},
		{
			name: "with --ignore-new",
			cmd: &diffCmd{
				Before:    path.Join(ts.testDir, "before.snap"),
				After:     path.Join(ts.testDir, "after.snap"),
				IgnoreNew: true,
			},
			testFunc: func(ts *testSuite, out *diffCmdOutput) {
				ts.Require().Equal(0, out.summary.new)
				ts.Require().Equal(1, out.summary.deleted)
				ts.Require().Equal(2, out.summary.modified)
				ts.Require().Len(out.changes, 3)
			},
		},
		{
			name: "with --ignore-modified",
			cmd: &diffCmd{
				Before:         path.Join(ts.testDir, "before.snap"),
				After:          path.Join(ts.testDir, "after.snap"),
				IgnoreModified: true,
			},
			testFunc: func(ts *testSuite, out *diffCmdOutput) {
				ts.Require().Equal(1, out.summary.new)
				ts.Require().Equal(1, out.summary.deleted)
				ts.Require().Equal(0, out.summary.modified)
				ts.Require().Len(out.changes, 2)
			},
		},
		{
			name: "with --ignore-deleted",
			cmd: &diffCmd{
				Before:        path.Join(ts.testDir, "before.snap"),
				After:         path.Join(ts.testDir, "after.snap"),
				IgnoreDeleted: true,
			},
			testFunc: func(ts *testSuite, out *diffCmdOutput) {
				ts.Require().Equal(1, out.summary.new)
				ts.Require().Equal(0, out.summary.deleted)
				ts.Require().Equal(2, out.summary.modified)
				ts.Require().Len(out.changes, 3)
			},
		},
		{
			name: "with --exclude",
			cmd: &diffCmd{
				Before:  path.Join(ts.testDir, "before.snap"),
				After:   path.Join(ts.testDir, "after.snap"),
				Exclude: []string{"c"},
			},
			testFunc: func(ts *testSuite, out *diffCmdOutput) {
				ts.Require().Equal(1, out.summary.new)
				ts.Require().Equal(1, out.summary.deleted)
				ts.Require().Equal(1, out.summary.modified)
				ts.Require().Len(out.changes, 3)

				ts.Require().Equal("a", func() fileDiff {
					for _, d := range out.changes {
						if d.diffType == diffTypeModified {
							return d
						}
					}
					ts.T().Fatal("no modified files found in changes list")
					return fileDiff{}
				}().fileAfter.Path)
			},
		},
	}

	for _, tt := range tests {
		ts.T().Run(tt.name, func(t *testing.T) {
			out, err := tt.cmd.run()
			ts.Require().NoError(err)
			tt.testFunc(ts, &out)
		})
	}
}
