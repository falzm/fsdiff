package main

import (
	"os"
	"path"
	"testing"

	"github.com/falzm/fsdiff/internal/snapshot"
)

func (ts *testSuite) TestSnapshotCmd_run() {
	tests := []struct {
		name      string
		cmd       *snapshotCmd
		setupFunc func(*testSuite, *snapshotCmd)
		testFunc  func(*testSuite, *snapshotCmd)
		wantErr   bool
	}{
		{
			name: "with --output-file",
			cmd: &snapshotCmd{
				Root:       ts.rootDir,
				OutputFile: path.Join(ts.testDir, ts.randomString(10)+".snap"),
			},
			setupFunc: func(t *testSuite, _ *snapshotCmd) { ts.createDummyFile("x", []byte("x"), 0o644) },
			testFunc: func(ts *testSuite, cmd *snapshotCmd) {
				ts.Require().FileExists(cmd.OutputFile)
				snap, err := snapshot.Open(cmd.OutputFile)
				ts.Require().NoError(err)
				defer snap.Close()
				filesByPath, err := snap.FilesByPath()
				ts.Require().NoError(err)
				ts.Require().Len(filesByPath, 1)
			},
		},
		{
			name: "with --shallow",
			cmd: &snapshotCmd{
				Root:       ts.rootDir,
				OutputFile: path.Join(ts.testDir, ts.randomString(10)+".snap"),
				Shallow:    true,
			},
			setupFunc: func(t *testSuite, _ *snapshotCmd) { ts.createDummyFile("x", []byte("x"), 0o644) },
			testFunc: func(ts *testSuite, cmd *snapshotCmd) {
				ts.Require().FileExists(cmd.OutputFile)
				snap, err := snapshot.Open(cmd.OutputFile)
				ts.Require().NoError(err)
				defer snap.Close()
				ts.True(snap.Metadata().Shallow)
			},
		},
		{
			name: "with --exclude",
			cmd: &snapshotCmd{
				Root:       ts.rootDir,
				OutputFile: path.Join(ts.testDir, ts.randomString(10)+".snap"),
				Exclude:    []string{"b"},
			},
			setupFunc: func(t *testSuite, _ *snapshotCmd) {
				ts.createDummyFile("a", []byte("a"), 0o644)
				ts.createDummyFile("b", []byte("b"), 0o644)
			},
			testFunc: func(ts *testSuite, cmd *snapshotCmd) {
				ts.Require().FileExists(cmd.OutputFile)
				snap, err := snapshot.Open(cmd.OutputFile)
				ts.Require().NoError(err)
				defer snap.Close()
				filesByPath, err := snap.FilesByPath()
				ts.Require().NoError(err)
				ts.Require().Len(filesByPath, 1)
				ts.Require().Equal("a", filesByPath[0].Path)
			},
		},
		{
			name: "with --exclude-from",
			cmd: &snapshotCmd{
				Root:        ts.rootDir,
				OutputFile:  path.Join(ts.testDir, ts.randomString(10)+".snap"),
				ExcludeFrom: path.Join(ts.testDir, ts.randomString(10)+".excludes"),
			},
			setupFunc: func(t *testSuite, cmd *snapshotCmd) {
				ts.createDummyFile("a", []byte("a"), 0o644)
				ts.createDummyFile("b", []byte("b"), 0o644)
				ts.Require().NoError(os.WriteFile(cmd.ExcludeFrom, []byte("b"), 0o644))
			},
			testFunc: func(ts *testSuite, cmd *snapshotCmd) {
				ts.Require().FileExists(cmd.OutputFile)
				snap, err := snapshot.Open(cmd.OutputFile)
				ts.Require().NoError(err)
				defer snap.Close()
				filesByPath, err := snap.FilesByPath()
				ts.Require().NoError(err)
				ts.Require().Len(filesByPath, 1)
				ts.Require().Equal("a", filesByPath[0].Path)
			},
		},
		{
			name: "filesystem error without --carry-on",
			cmd: &snapshotCmd{
				Root:       ts.rootDir,
				OutputFile: path.Join(ts.testDir, ts.randomString(10)+".snap"),
			},
			setupFunc: func(t *testSuite, _ *snapshotCmd) { ts.createDummyFile("x", []byte("x"), 0o000) },
			wantErr:   true,
		},
		{
			name: "filesystem error with --carry-on",
			cmd: &snapshotCmd{
				Root:       ts.rootDir,
				OutputFile: path.Join(ts.testDir, ts.randomString(10)+".snap"),
				CarryOn:    true,
			},
			setupFunc: func(t *testSuite, _ *snapshotCmd) { ts.createDummyFile("x", []byte("x"), 0o000) },
			testFunc: func(ts *testSuite, cmd *snapshotCmd) {
				ts.Require().FileExists(cmd.OutputFile)
				snap, err := snapshot.Open(cmd.OutputFile)
				ts.Require().NoError(err)
				defer snap.Close()
				filesByPath, err := snap.FilesByPath()
				ts.Require().NoError(err)
				ts.Require().Len(filesByPath, 0)
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
				tt.setupFunc(ts, tt.cmd)
			}

			err = tt.cmd.Run()
			if (err != nil) != tt.wantErr {
				t.Errorf("snapshotCmd.Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			tt.testFunc(ts, tt.cmd)
		})
	}
}
