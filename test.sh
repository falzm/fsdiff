#!/usr/bin/env bash

function die() {
    echo "error: $*" >&2
    exit 1
}

[[ ${BASH_VERSINFO[0]} -ge 4 ]] || die "tests require Bash >= 4.0"

TMPDIR="$(mktemp -d)"
TESTROOTDIR="$TMPDIR/root/"
trap cleanup EXIT

function setup() {
    mkdir "$TESTROOTDIR" ; cd "$TESTROOTDIR"
    mkdir a
    echo b > a/b
    mkdir a/c
    echo d > a/c/d
    echo z > z
}

function teardown() {
    rm -rf "$TESTROOTDIR"
}

function cleanup() {
    cd / && rm -rf "$TMPDIR"
}

function pass() {
    echo "😎 Test ${FUNCNAME[1]/test_} passed"
}

function fail() {
    echo -e "💥 Test ${FUNCNAME[1]/test_} failed: $*"
    exit 1
}

function skip() {
    echo "🙈 Test ${FUNCNAME[1]/test_} skipped"
}

function run_test() {
    setup || die "unable to setup test environment"

    echo "🚧 Running test ${1/test_}"
    $1 

    teardown

    rm -rf $out
}

function main() {
    # Read this script looking for functions named test_* and run them sequentially
    grep -E -o '^test_[a-z0-9_]*' $(readlink -f $0) | while read test; do
        run_test $test
        echo
    done
}

##############################################################################
#                                  T E S T S                                 #
##############################################################################

test_snapshot_rootdir() {
    fsdiff snapshot -o "$TMPDIR/snap" "$TESTROOTDIR" ; rc=$?
    [[ $rc -eq 0 ]] || fail "expected rc 0, got $rc"
    [[ -e "$TMPDIR/snap" ]] || fail "snapshot file not created"

    pass
}

test_snapshot_symlinked_rootdir() {
    ln -s "$TESTROOTDIR" "$TMPDIR/root_l"
    fsdiff snapshot -o "$TMPDIR/snap" "$TMPDIR/root_l" ; rc=$?
    [[ $rc -eq 0 ]] || fail "expected rc 0, got $rc"
    [[ -e "$TMPDIR/snap" ]] || fail "snapshot file not created"

    pass
}

test_snapshot_rootdir_carry_on_error() {
    chmod 000 "$TESTROOTDIR/a"
    fsdiff snapshot --carry-on -o "$TMPDIR/snap" "$TESTROOTDIR" ; rc=$?
    [[ $rc -eq 0 ]] || fail "expected rc 0, got $rc"
    [[ -e "$TMPDIR/snap" ]] || fail "snapshot file not created"

    chmod 755 "$TESTROOTDIR/a" # Restore original permissions otherwise teardown will fail

    pass
}

test_dump_snapshot() {
    fsdiff snapshot -o "$TMPDIR/snap" "$TESTROOTDIR"
    fsdiff dump "$TMPDIR/snap" 1> "$TMPDIR/out"
    [[ $? -eq 0 ]] || fail "return code is not 0"

    by_path=$(sed -ne '/## by_path/,/^## by_cs/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_path -eq 5 ]] || fail "expected 5 entries in section by_path, got $by_path"

    by_cs=$(sed -ne '/## by_cs/,/^## metadata/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_cs -eq 3 ]] || fail "expected 3 entries in section by_path, got $by_cs"

    pass
}

test_snapshot_with_exclude_flag() {
    fsdiff snapshot -o "$TMPDIR/snap" --exclude a "$TESTROOTDIR"
    fsdiff dump "$TMPDIR/snap" 1> "$TMPDIR/out"
    [[ $? -eq 0 ]] || fail "return code is not 0"

    by_path=$(sed -ne '/## by_path/,/^## by_c/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_path -eq 1 ]] || fail "expected 1 entry in section by_path, got $by_path"

    by_cs=$(sed -ne '/## by_cs/,/^## metadata/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_cs -eq 1 ]] || fail "expected 1 entry in section by_path, got $by_cs"

    pass
}

test_snapshot_with_exclude_from() {
    echo a > "$TMPDIR/exclude"
    fsdiff snapshot -o "$TMPDIR/snap" --exclude-from "$TMPDIR/exclude" "$TESTROOTDIR"
    fsdiff dump "$TMPDIR/snap" 1> "$TMPDIR/out"
    [[ $? -eq 0 ]] || fail "return code is not 0"

    by_path=$(sed -ne '/## by_path/,/^## by_cs/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_path -eq 1 ]] || fail "expected 1 entry in section by_path, got $by_path"

    by_cs=$(sed -ne '/## by_cs/,/^## metadata/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_cs -eq 1 ]] || fail "expected 1 entry in section by_path, got $by_cs"

    pass
}

test_snapshot_with_exclude_flag_and_from() {
    echo a > "$TMPDIR/exclude"
    fsdiff snapshot -o "$TMPDIR/snap" --exclude-from "$TMPDIR/exclude" --exclude z "$TESTROOTDIR"
    fsdiff dump "$TMPDIR/snap" 1> "$TMPDIR/out"
    [[ $? -eq 0 ]] || fail "return code is not 0"

    by_path=$(sed -ne '/## by_path/,/^## by_cs/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_path -eq 0 ]] || fail "expected 0 entry in section by_path, got $by_path"

    by_cs=$(sed -ne '/## by_cs/,/^## metadata/p' "$TMPDIR/out" | grep -v '#' | wc -l)
    [[ $by_cs -eq 0 ]] || fail "expected 0 entry in section by_path, got $by_cs"

    pass
}

test_diff_containing_symlinks() {
    ln -s a "$TESTROOTDIR/a_l"
    ln -s z "$TESTROOTDIR/z_l"
    fsdiff snapshot -o "$TMPDIR/snap" "$TESTROOTDIR" ; rc=$?
    fsdiff dump "$TMPDIR/snap" 1> "$TMPDIR/out"
    grep -Eq -e "^a_l.*link:a$" -e "^z_l.*link:z$" "$TMPDIR/out" || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_without_changes() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    [[ "$(<$TMPDIR/out)" == "" ]] || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_new_file() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    echo x > "$TESTROOTDIR/x"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    grep -Pzq "^\+ x\s+1 new, 0 changed, 0 deleted" "$TMPDIR/out" || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_deleted_file() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    rm -rf "$TESTROOTDIR/z"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    grep -Pzq "^\- z\s+0 new, 0 changed, 1 deleted" "$TMPDIR/out" || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_modified_file() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    echo zz > "$TESTROOTDIR/z"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    grep -Pzq "^\~ z\
\s+size:2.*checksum:3a710d2a84f856bc4e1c0bbb93ca517893c48691\
\s+size:3.*checksum:15546de8c3b03e70ceec10a49f271b96b745a0a6\
\s+0 new, 1 changed, 0 deleted" "$TMPDIR/out" || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_modified_symlink() {
    ln -s a "$TESTROOTDIR/a_l"
    ln -s z "$TESTROOTDIR/z_l"
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    rm -f "$TESTROOTDIR"/*_l
    ln -s z "$TESTROOTDIR/a_l"
    ln -s a "$TESTROOTDIR/z_l"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    grep -Pzq "^\~ a_l\s.*link:a\s.*link:z\s+\
\~ z_l\s.*link:z\s.*link:a\s+\
0 new, 2 changed, 0 deleted" "$TMPDIR/out" || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_renamed_file() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    mv "$TESTROOTDIR/z" "$TESTROOTDIR/zz"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    grep -Pzq "^\> z => zz\s+0 new, 1 changed, 0 deleted" "$TMPDIR/out" || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_ignored_mtime_change() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    sleep 1s; touch "$TESTROOTDIR/z"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor \
        --ignore mtime \
        "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    [[ "$(<$TMPDIR/out)" == "" ]] || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_ignored_cs_change() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    echo . > "$TESTROOTDIR/z"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor \
        --ignore mtime \
        --ignore checksum \
        "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    [[ "$(<$TMPDIR/out)" == "" ]] || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_ignored_size_change() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    echo zzz > "$TESTROOTDIR/z"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor \
        --ignore mtime \
        --ignore checksum \
        --ignore size \
        "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    [[ "$(<$TMPDIR/out)" == "" ]] || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_ignored_mode_change() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    chmod 777 "$TESTROOTDIR/z"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor \
        --ignore mtime \
        --ignore checksum \
        --ignore size \
        --ignore mode \
        "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    [[ "$(<$TMPDIR/out)" == "" ]] || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_shallow_mode() {
    fsdiff snapshot --shallow -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    echo . > "$TESTROOTDIR/a/c/d"
    fsdiff snapshot --shallow -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor \
        --ignore mtime \
        "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    [[ "$(<$TMPDIR/out)" == "" ]] || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_with_exclude_flag() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    echo . > "$TESTROOTDIR/a/c/d"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor \
        --exclude a/c/ \
        "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    [[ "$(<$TMPDIR/out)" == "" ]] || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

test_diff_summary_only() {
    fsdiff snapshot -o "$TMPDIR/before.snap" "$TESTROOTDIR"
    echo x > "$TESTROOTDIR/x"
    echo . > "$TESTROOTDIR/a/c/d"
    rm -rf "$TESTROOTDIR/z"
    fsdiff snapshot -o "$TMPDIR/after.snap" "$TESTROOTDIR"
    fsdiff diff --nocolor \
        --ignore mtime \
        --summary \
        "$TMPDIR/before.snap" "$TMPDIR/after.snap" > "$TMPDIR/out"
    grep -Pzq "^1 new, 1 changed, 1 deleted" "$TMPDIR/out" || fail "unexpected output:\n$(<$TMPDIR/out)"

    pass
}

##############################################################################

which fsdiff > /dev/null || die "unable to find fsdiff command"

main
