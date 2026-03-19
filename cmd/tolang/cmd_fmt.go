package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tos-network/tolang/tol/format"
)

// cmdFormat implements the `tol fmt` subcommand.
//
// Usage:
//
//	tol fmt [flags] <files or dirs...>
//	tol fmt file.tol          — print formatted source to stdout
//	tol fmt -w file.tol       — format in place
//	tol fmt -l file.tol       — list files that would change
//	tol fmt dir/              — format all .tol files in directory
//
// Exit codes:
//
//	0 - success (or no changes with -l)
//	1 - error
//	2 - files would change (with -l)
func cmdFormat(args []string) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var writeInPlace, listOnly bool
	fs.BoolVar(&writeInPlace, "w", false, "write formatted output back to source files")
	fs.BoolVar(&listOnly, "l", false, "list files whose formatting differs from tol fmt")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol fmt [-w] [-l] <file.tol|dir> ...")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "fmt requires at least one file or directory argument")
		fs.Usage()
		return 1
	}

	// Collect all .tol files from arguments.
	var files []string
	for _, arg := range fs.Args() {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if info.IsDir() {
			dirFiles, err := collectTolFiles(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
			files = append(files, dirFiles...)
		} else {
			files = append(files, arg)
		}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no .tol files found")
		return 1
	}

	hasChanges := false
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
			return 1
		}

		formatted, err := format.Format(src, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error formatting %s: %v\n", path, err)
			return 1
		}

		if listOnly {
			if !bytes.Equal(src, formatted) {
				fmt.Println(path)
				hasChanges = true
			}
			continue
		}

		if writeInPlace {
			if !bytes.Equal(src, formatted) {
				if err := os.WriteFile(path, formatted, 0o644); err != nil {
					fmt.Fprintf(os.Stderr, "error writing %s: %v\n", path, err)
					return 1
				}
			}
			continue
		}

		// Default: print to stdout.
		os.Stdout.Write(formatted)
	}

	if listOnly && hasChanges {
		return 2
	}
	return 0
}

// collectTolFiles recursively collects all .tol files under a directory.
func collectTolFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".tol") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
