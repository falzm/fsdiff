package main

import (
	"os"
	"strings"
	"time"

	"github.com/falzm/fsdiff/internal/snapshot"
)

type snapshotCmd struct {
	Root string `arg:"" type:"existingdir" default:"." help:"Path to root directory."`

	CarryOn     bool     `help:"Continue on filesystem error."`
	Exclude     []string `placeholder:"PATTERN" help:"gitignore-compatible exclusion pattern (see https://git-scm.com/docs/gitignore)."`
	ExcludeFrom string   `type:"existingfile" help:"File path to read gitignore-compatible patterns from (see https://git-scm.com/docs/gitignore)."`
	OutputFile  string   `short:"o" help:"File path to write snapshot to (default: <YYYYMMDDhhmmss>.snap)."`
	Shallow     bool     `help:"Don't compute files checksum."`
}

func (c *snapshotCmd) Run() error {
	opts := make([]snapshot.CreateOpt, 0)

	if c.CarryOn {
		opts = append(opts, snapshot.CreateOptCarryOn())
	}

	if c.ExcludeFrom != "" {
		data, err := os.ReadFile(c.ExcludeFrom)
		if err != nil {
			return err
		}
		c.Exclude = append(c.Exclude, strings.Split(string(data), "\n")...)
	}
	opts = append(opts, snapshot.CreateOptExclude(c.Exclude))

	if c.Shallow {
		opts = append(opts, snapshot.CreateOptShallow())
	}

	if c.OutputFile == "" {
		c.OutputFile = time.Now().Format("20060102150405.snap")
	}

	snap, err := snapshot.Create(c.OutputFile, c.Root, opts...)
	if err != nil {
		return err
	}

	return snap.Close()
}
