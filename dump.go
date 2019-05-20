package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	bolt "go.etcd.io/bbolt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	cmdDump            = kingpin.Command("dump", "Dump snapshot information")
	cmdDumpArgSnapshot = cmdDump.Arg("snapshot", "Path to snapshot file").
				Required().
				ExistingFile()
)

func dump(path string) error {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	return db.View(func(tx *bolt.Tx) error {
		pathBucket := tx.Bucket([]byte("by_path"))
		if pathBucket == nil {
			return errors.New(`"by_path" bucket not found in snapshot file`)
		}

		fmt.Printf("## by_path (%d)\n", pathBucket.Stats().KeyN)
		c := pathBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			fi := fileInfo{}
			unmarshal(v, &fi)
			fmt.Printf("%s %s\n", k, fi.String())
		}

		csBucket := tx.Bucket([]byte("by_cs"))
		if csBucket == nil {
			return errors.New(`"by_cs" bucket not found in snapshot file`)
		}

		fmt.Printf("## by_cs (%d)\n", csBucket.Stats().KeyN)
		c = csBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			fi := fileInfo{}
			unmarshal(v, &fi)
			fmt.Printf("%x %s\n", k, fi.String())
		}

		metaBucket := tx.Bucket([]byte("metadata"))
		if metaBucket == nil {
			return errors.New(`"metadata" bucket not found in snapshot file`)
		}

		data := metaBucket.Get([]byte("info"))
		if data == nil {
			return errors.New("invalid snapshot metadata")
		}

		meta := metadata{}
		unmarshal(data, &meta)
		fmt.Printf("## metadata\nformat version: %d\nfsdiff version: %s\ndate: %s\nroot: %s\n",
			meta.FormatVersion,
			meta.FsdiffVersion,
			meta.Date,
			meta.RootDir)

		return nil
	})
}
