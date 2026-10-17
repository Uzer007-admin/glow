package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Keep unchanged native inputs untouched. Stage individual writes so a missing
// source or a failed write cannot truncate the previous working header/stub.
func writeFileIfChanged(path string, data []byte, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("generated destination is not a regular file: %s", path)
		}
		previous, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Equal(previous, data) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".glow-file-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
