package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSymbolManifestIsDeterministic(t *testing.T) {
	clear, viewport := &PackageFunction{Required: true}, &PackageFunction{}
	clear.GoName, viewport.GoName = "Clear", "Viewport"
	clear.Return = Type{Name: "void", CDefinition: "void"}
	clear.Parameters = []Parameter{{Name: "mask", Type: Type{Name: "GLbitfield", CDefinition: "GLbitfield"}}}
	pkg := &Package{API: "gl", Version: Version{2, 1}, Profile: "core",
		Functions: map[string]*PackageFunction{"glViewport": viewport, "glClear": clear},
		Enums:     map[string]*Enum{"GL_TRUE": {}, "GL_FALSE": {}}}
	dir := t.TempDir()
	if err := pkg.writeManifest(dir); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "symbol_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.writeManifest(dir); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dir, "symbol_manifest.json"))
	if err != nil || string(first) != string(second) {
		t.Fatalf("non-deterministic manifest: %v", err)
	}
	text := string(first)
	var manifest struct {
		Schema    int
		Functions []manifestFunction
	}
	if err := json.Unmarshal(first, &manifest); err != nil || manifest.Schema != 2 ||
		manifest.Functions[0].Return.Go != "" || manifest.Functions[0].Parameters[0].Type.Go != "uint32" {
		t.Fatalf("signature metadata lost: %+v; %v", manifest, err)
	}
	if strings.Index(text, "glClear") >= strings.Index(text, "glViewport") ||
		strings.Index(text, "GL_FALSE") >= strings.Index(text, "GL_TRUE") ||
		!strings.Contains(text, `"required": true`) || !strings.Contains(text, `"version": "2.1"`) {
		t.Fatalf("invalid ordered symbol metadata: %s", text)
	}
}

func TestPublicationPreservesUnchangedFileTimestamp(t *testing.T) {
	dir, staging := t.TempDir(), t.TempDir()
	data := []byte("package gl\n")
	for _, directory := range []string{dir, staging} {
		if err := os.WriteFile(filepath.Join(directory, "package.go"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "package.go")
	stamp := time.Unix(1700000000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := publishGenerated(dir, staging, []string{"package.go"}, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Fatalf("unchanged file republished: %v; %v", info, err)
	}
}

func TestPublicationFailureRestoresPreviousFiles(t *testing.T) {
	dir, staging := t.TempDir(), t.TempDir()
	previous := []byte("previous working package\n")
	if err := os.WriteFile(filepath.Join(dir, "package.go"), previous, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "package.go"), []byte("replacement\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// The second missing staged file forces an actual rename failure after the
	// first destination was replaced; rollback must restore the working version.
	err := publishGenerated(dir, staging, []string{"package.go", "conversions.go"}, false)
	if err == nil {
		t.Fatal("missing staged file accepted")
	}
	actual, err := os.ReadFile(filepath.Join(dir, "package.go"))
	if err != nil || string(actual) != string(previous) {
		t.Fatalf("publication rollback failed: %q; %v", actual, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "conversions.go")); !os.IsNotExist(err) {
		t.Fatalf("failed publication created an unexpected file: %v", err)
	}
}
