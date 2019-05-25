# fsdiff

`fsdiff` is a simple tool that helps finding out what changes occurred in a filesystem tree.

Using `fsdiff` involves two steps: first take a `snapshot` of the target filesystem *before* modifications happen,
then another one *after*. Then, the `diff` step compares the two snapshots and reports the changes found.

Here is an example illustrating how to use this tool. We start with an existing file tree:

```console
$ tree test/
test
├── a
│   ├── b
│   └── c
│       └── d
└── z

2 directories, 3 files
```

We take a snapshot of the current state, which will be our reference point in case of later modification:

```console
$ fsdiff snapshot -o before.snap test/
```

Now, let's shake things up:

```console
$ mkdir test/.x
$ echo y > test/.x/y
$ rm -rf test/a/c/
$ echo z > test/a/b
```

We now take a second snapshot representing the current state:

```console
$ fsdiff snapshot -o after.snap test/
```

To know what happened during the two snapshots, we run a *diff* operation:

```console
$ fsdiff diff before.snap after.snap
+ .x
+ .x/y
~ a
  size:128 mtime:2019-05-19 20:30:22.638232884 +0200 CEST uid:501 gid:20 mode:drwxr-xr-x DIR
  size:96 mtime:2019-05-19 20:33:26.626202277 +0200 CEST uid:501 gid:20 mode:drwxr-xr-x DIR
~ a/b
  size:2 mtime:2019-05-19 20:30:22.528923845 +0200 CEST uid:501 gid:20 mode:-rw-r--r-- checksum:89e6c98d92887913cadf06b2adb97f26cde4849b
  size:2 mtime:2019-05-19 20:33:38.893340925 +0200 CEST uid:501 gid:20 mode:-rw-r--r-- checksum:3a710d2a84f856bc4e1c0bbb93ca517893c48691
- a/c
- a/c/d

2 new, 2 changed, 2 deleted
```


## Installation

### Pre-compiled binaries

Pre-compiled binaries for GNU/Linux and macOS are available for [stable releases](https://github.com/falzm/fsdiff/releases).

### Using Homebrew

On macOS, you can use the [Homebrew](https://brew.sh/) packaging system:

```console
brew tap falzm/fsdiff
brew install fsdiff
```

### Using `go get`

```console
go get github.com/falzm/fsdiff
```

### From source (requires a Go compiler >= 1.9)

At the top of the sources directory, just type `make`. If everything went well, you should end up with binary named `fsdiff` in your current directory.


## Usage

Usage documentation is available by running the `fsdiff help` command.

Note: when performing a `diff` operation, the command will exit with a return code 2 if changes have been detected
between two snapshots, and 0 if no changes. Any other error returns 1.

### Shallow mode

`fsdiff` supports a *shallow* mode, in which files checksum are not computed. This can be useful if snapshotting very
large file trees and/or large files as the operation will be much less resource-intensive, at the cost of a less
precise change tracking: in *shallow* mode it is not possible to detect file renamings or content-focused file
changes anymore.

To use *shallow* mode, set the `--shallow` command flag during a *snapshot* operation. Note: during a
*diff* operation, if `fsdiff` detects that either one of the snapshots is *shallow* the operation will be performed
in *shallow mode* too.
