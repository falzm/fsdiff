package main

import (
	"fmt"

	"github.com/alecthomas/kong"

	"github.com/falzm/fsdiff/internal/snapshot"
)

type dumpCmdOutput struct {
	filesByChecksum []*snapshot.FileInfo
	filesByPath     []*snapshot.FileInfo
	metadata        *snapshot.Metadata
}

type dumpCmd struct {
	SnapshotFile string `arg:"" name:"snapshot" type:"existingfile" help:"Path to snapshot file."`

	MetadataOnly bool `name:"metadata" help:"Only dump snapshot metadata."`
}

func (c *dumpCmd) run() (dumpCmdOutput, error) {
	var out dumpCmdOutput

	snap, err := snapshot.Open(c.SnapshotFile)
	if err != nil {
		return dumpCmdOutput{}, fmt.Errorf("unable to open snapshot file: %w", err)
	}
	defer snap.Close()

	if out.filesByChecksum, err = snap.FilesByChecksum(); err != nil {
		return dumpCmdOutput{}, err
	}
	if out.filesByPath, err = snap.FilesByPath(); err != nil {
		return dumpCmdOutput{}, err
	}

	out.metadata = snap.Metadata()

	return out, nil
}

func (c *dumpCmd) Run(ctx kong.Context) error {
	out, err := c.run()
	if err != nil {
		return err
	}

	if !c.MetadataOnly {
		_, _ = fmt.Fprintf(ctx.Stdout, "## by_path (%d)\n", len(out.filesByPath))
		for _, fi := range out.filesByPath {
			_, _ = fmt.Fprintf(ctx.Stdout, "%s %s\n", fi.Path, fi.String())
		}

		_, _ = fmt.Fprintf(ctx.Stdout, "## by_cs (%d)\n", len(out.filesByChecksum))
		for _, fi := range out.filesByChecksum {
			_, _ = fmt.Fprintf(ctx.Stdout, "%s %s\n", fi.Path, fi.String())
		}
	}

	_, _ = fmt.Fprintf(
		ctx.Stdout,
		"## metadata\nformat version: %d\nfsdiff version: %s\ndate: %s\nroot: %s\nshallow: %t\n",
		out.metadata.FormatVersion,
		out.metadata.FsdiffVersion,
		out.metadata.Date,
		out.metadata.RootDir,
		out.metadata.Shallow,
	)

	return nil
}
