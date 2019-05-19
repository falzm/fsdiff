package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"time"
)

type fileInfo struct {
	Path     string
	Size     int64
	Mtime    time.Time
	Uid      uint32
	Gid      uint32
	Mode     os.FileMode
	IsDir    bool
	Checksum []byte
}

func (f *fileInfo) String() string {
	// The `Path` property is not displayed, as only used in reverse lookup to track file renaming

	s := fmt.Sprintf("size:%d mtime:%s uid:%d gid:%d mode:%v",
		f.Size,
		f.Mtime,
		f.Uid,
		f.Gid,
		f.Mode,
	)

	if f.IsDir {
		return s + " DIR"
	}

	return fmt.Sprintf("%s checksum:%x", s, f.Checksum)
}

func checksumFile(path string) ([]byte, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cs := sha1.Sum(data)

	bytes := make([]byte, len(cs))
	for i := range cs {
		bytes[i] = cs[i]
	}

	return bytes, nil
}

func compare(before, after *fileInfo, ignore []string) map[string][2]interface{} {
	diff := make(map[string][2]interface{})

	if !ignored("size", ignore) {
		if before.Size != after.Size {
			diff["size"] = [2]interface{}{before.Size, after.Size}
		}
	}

	if !ignored("mtime", ignore) {
		if !before.Mtime.Equal(after.Mtime) {
			diff["mtime"] = [2]interface{}{before.Mtime, after.Mtime}
		}
	}

	if !ignored("uid", ignore) {
		if before.Uid != after.Uid {
			diff["uid"] = [2]interface{}{before.Uid, after.Uid}
		}
	}

	if !ignored("gid", ignore) {
		if before.Gid != after.Gid {
			diff["gid"] = [2]interface{}{before.Gid, after.Gid}
		}
	}

	if !ignored("mode", ignore) {
		if before.Mode != after.Mode {
			diff["mode"] = [2]interface{}{before.Mode, after.Mode}
		}
	}

	if before.IsDir != after.IsDir {
		diff["dir"] = [2]interface{}{before.IsDir, after.IsDir}
	}

	if !ignored("checksum", ignore) {
		if !bytes.Equal(before.Checksum, after.Checksum) {
			diff["checksum"] = [2]interface{}{before.Checksum, after.Checksum}
		}
	}

	return diff
}

// ignored returns true if a property p is in the ignored list, otherwise false.
func ignored(p string, ignored []string) bool {
	for i := range ignored {
		if ignored[i] == p {
			return true
		}
	}

	return false
}
