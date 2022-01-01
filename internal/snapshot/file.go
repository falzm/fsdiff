package snapshot

import (
	"crypto/sha1"
	"fmt"
	"os"
	"time"
)

// FileInfo represents information about a file referenced in a Snapshot.
type FileInfo struct {
	Path     string
	Size     int64
	Mtime    time.Time
	Uid      uint32 // FIXME: rename field to "UID" during next snapshot format version increment
	Gid      uint32 // FIXME: rename field to "GID" during next snapshot format version increment
	Mode     os.FileMode
	LinkTo   string
	IsDir    bool
	IsSock   bool
	IsPipe   bool
	IsDev    bool
	Checksum []byte
}

// String implements the fmt.Stringer interface.
func (f *FileInfo) String() string {
	// The `Path` property is not displayed, as only used in reverse lookup to track file renaming.

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

	if f.IsSock {
		return s + " SOCK"
	}

	if f.IsPipe {
		return s + " PIPE"
	}

	if f.IsDev {
		return s + " DEV"
	}

	if f.LinkTo != "" {
		return fmt.Sprintf("%s link:%s", s, f.LinkTo)
	}

	if f.Checksum != nil {
		return fmt.Sprintf("%s checksum:%x", s, f.Checksum)
	}

	return s
}

func checksumFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
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
