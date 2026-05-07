package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("archive-state", flag.ContinueOnError)
	fl.SetOutput(stdout)
	src := fl.String("src", "data/state", "source state directory")
	dstBase := fl.String("dst-base", "data/state-archive", "archive base directory")
	if err := fl.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	fi, err := os.Stat(*src)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("source directory does not exist: %w", err)
		}
		return fmt.Errorf("stat source: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("source is not a directory: %s", *src)
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	archiveDir := filepath.Join(*dstBase, ts)

	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}

	if err := filepath.WalkDir(*src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(*src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dstPath := filepath.Join(archiveDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		return copyFile(path, dstPath)
	}); err != nil {
		return fmt.Errorf("archive copy: %w", err)
	}

	fmt.Fprintln(stdout, archiveDir)
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}
