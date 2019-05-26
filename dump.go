package main

import (
	"fmt"

	"github.com/falzm/fsdiff/snapshot"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	cmdDump             = kingpin.Command("dump", "Dump snapshot information")
	cmdDumpFlagMetadata = cmdDump.Flag("metadata", "Only dump snapshot metadata").
				Bool()
	cmdDumpArgSnapshot = cmdDump.Arg("snapshot", "Path to snapshot file").
				Required().
				ExistingFile()
)

func doDump() error {
	var (
		path         = *cmdDumpArgSnapshot
		metadataOnly = *cmdDumpFlagMetadata
	)

	snap, err := snapshot.Open(path)
	if err != nil {
		return errors.Wrap(err, "unable to open snapshot file")
	}
	defer snap.Close()

	return snap.Read(func(byPath, byCS *bolt.Bucket) error {
		if !metadataOnly {
			fmt.Printf("## by_path (%d)\n", byPath.Stats().KeyN)
			c := byPath.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				fi := fileInfo{}
				if err := snapshot.Unmarshal(v, &fi); err != nil {
					return errors.Wrap(err, "unable to read snapshot data")
				}
				fmt.Printf("%s %s\n", k, fi.String())
			}

			fmt.Printf("## by_cs (%d)\n", byCS.Stats().KeyN)
			c = byCS.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				fi := fileInfo{}
				if err := snapshot.Unmarshal(v, &fi); err != nil {
					return errors.Wrap(err, "unable to read snapshot data")
				}
				fmt.Printf("%x %s\n", k, fi.String())
			}
		}

		meta := snap.Metadata()
		fmt.Printf("## metadata\nformat version: %d\nfsdiff version: %s\ndate: %s\nroot: %s\nshallow: %t\n",
			meta.FormatVersion,
			meta.FsdiffVersion,
			meta.Date,
			meta.RootDir,
			meta.Shallow)

		return nil
	})
}
