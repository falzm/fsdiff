package snapshot

import (
	"crypto/sha1"
	"fmt"
	"os"
	"testing"
	"time"
)

func (ts *testSuite) TestFileInfo_String() {
	var (
		testChecksum = func() []byte {
			sha1sum := sha1.Sum([]byte(ts.randomString(10)))
			bytes := make([]byte, len(sha1sum))
			for i := range sha1sum {
				bytes[i] = sha1sum[i]
			}
			return bytes
		}()
		testGID      uint32      = 2000
		testLinkTo               = ts.randomString(10)
		testModeDir  os.FileMode = 0o755
		testModeFile os.FileMode = 0o644
		testMtime                = time.Now()
		testSize     int64       = 42
		testUID      uint32      = 1000
	)

	tests := []struct {
		name     string
		fileInfo *FileInfo
		want     string
	}{
		{
			name: "regular file",
			fileInfo: &FileInfo{
				Size:     testSize,
				Mtime:    testMtime,
				Uid:      testUID,
				Gid:      testGID,
				Mode:     testModeFile,
				Checksum: testChecksum,
			},
			want: fmt.Sprintf("size:%d mtime:%s uid:%d gid:%d mode:%v checksum:%x",
				testSize,
				testMtime,
				testUID,
				testGID,
				testModeFile,
				testChecksum,
			),
		},
		{
			name: "directory",
			fileInfo: &FileInfo{
				Size:  testSize,
				Mtime: testMtime,
				Uid:   testUID,
				Gid:   testGID,
				Mode:  testModeDir,
				IsDir: true,
			},
			want: fmt.Sprintf("size:%d mtime:%s uid:%d gid:%d mode:%v DIR",
				testSize,
				testMtime,
				testUID,
				testGID,
				testModeDir,
			),
		},
		{
			name: "socket",
			fileInfo: &FileInfo{
				Size:   testSize,
				Mtime:  testMtime,
				Uid:    testUID,
				Gid:    testGID,
				Mode:   testModeFile,
				IsSock: true,
			},
			want: fmt.Sprintf("size:%d mtime:%s uid:%d gid:%d mode:%v SOCK",
				testSize,
				testMtime,
				testUID,
				testGID,
				testModeFile,
			),
		},
		{
			name: "pipe",
			fileInfo: &FileInfo{
				Size:   testSize,
				Mtime:  testMtime,
				Uid:    testUID,
				Gid:    testGID,
				Mode:   testModeFile,
				IsPipe: true,
			},
			want: fmt.Sprintf("size:%d mtime:%s uid:%d gid:%d mode:%v PIPE",
				testSize,
				testMtime,
				testUID,
				testGID,
				testModeFile,
			),
		},
		{
			name: "device",
			fileInfo: &FileInfo{
				Size:  testSize,
				Mtime: testMtime,
				Uid:   testUID,
				Gid:   testGID,
				Mode:  testModeFile,
				IsDev: true,
			},
			want: fmt.Sprintf("size:%d mtime:%s uid:%d gid:%d mode:%v DEV",
				testSize,
				testMtime,
				testUID,
				testGID,
				testModeFile,
			),
		},
		{
			name: "symlink",
			fileInfo: &FileInfo{
				Size:   testSize,
				Mtime:  testMtime,
				Uid:    testUID,
				Gid:    testGID,
				Mode:   testModeFile,
				LinkTo: testLinkTo,
			},
			want: fmt.Sprintf("size:%d mtime:%s uid:%d gid:%d mode:%v link:%s",
				testSize,
				testMtime,
				testUID,
				testGID,
				testModeFile,
				testLinkTo,
			),
		},
	}

	for _, tt := range tests {
		ts.T().Run(tt.name, func(t *testing.T) {
			ts.Require().Equal(tt.want, tt.fileInfo.String())
		})
	}
}
