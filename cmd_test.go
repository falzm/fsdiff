package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
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

func TestFsdiffTestSuite(t *testing.T) {
	suite.Run(t, new(testSuite))
}
