package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeaderCopyFailureAndUnchangedTimestamp(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.h"), filepath.Join(dir, "target.h")
	data := []byte("typedef unsigned int GLenum;\n")
	if err := os.WriteFile(target, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(source, target); err == nil {
		t.Fatal("missing header source accepted")
	}
	actual, _ := os.ReadFile(target)
	if string(actual) != string(data) {
		t.Fatal("missing source truncated header")
	}
	if err := os.WriteFile(source, data, 0644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1700000000, 0)
	if err := os.Chtimes(target, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(source, target); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Fatal("unchanged header replaced", err)
	}
	staging, _ := filepath.Glob(filepath.Join(dir, ".glow-file-*"))
	if len(staging) != 0 {
		t.Fatal("header staging remains")
	}
}
