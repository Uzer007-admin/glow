package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateContextDispatch(t *testing.T) {
	pkg := &Package{Name: "gl", API: "gl", Version: Version{2, 1}, TmplDir: "tmpl", ContextDispatch: true,
		Functions: map[string]*PackageFunction{"glFinish": {Function: Function{Name: "glFinish", GoName: "Finish", Return: Type{Name: "void", CDefinition: "void"}}, Required: true}}}
	out := t.TempDir()
	if err := pkg.GeneratePackage(out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "package.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"type Functions struct", "func LoadWithProcAddrFunc", "func (glowContext *Functions) Finish()", "C.glowFinish(glowContext.gpFinish)", "return nil, errors.New(firstMissing)"} {
		if !strings.Contains(string(data), fragment) {
			t.Fatalf("missing %s", fragment)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(out, "symbol_manifest.json"))
	if err != nil || !strings.Contains(string(manifest), `"context_dispatch": true`) {
		t.Fatalf("manifest: %s %v", manifest, err)
	}
}

func TestDispatchReceiverAvoidsParameterNames(t *testing.T) {
	parameters := []Parameter{{Name: "f"}, {Name: "glowContext"}, {Name: "glowContext_"}}
	if name := (Function{Parameters: parameters}).DispatchReceiver(); name != "glowContext__" {
		t.Fatal(name)
	}
	if name := (Overload{Parameters: parameters}).DispatchReceiver(); name != "glowContext__" {
		t.Fatal(name)
	}
}

func TestContextDispatchRejectsGlobalDebugCallback(t *testing.T) {
	pkg := &Package{ContextDispatch: true, Functions: map[string]*PackageFunction{"glDebugMessageCallback": {Function: Function{Parameters: []Parameter{{Name: "callback", Type: Type{Name: "GLDEBUGPROC"}}}}}}}
	path := filepath.Join(t.TempDir(), "must-not-publish")
	if err := pkg.GeneratePackage(path); err == nil || !strings.Contains(err.Error(), "global user data") {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unsafe dispatch touched output before validation")
	}
}

func TestFixedSizeQueryBridgesAreDispatchOnly(t *testing.T) {
	registry, err := readSpecFile(filepath.Join("xml", "spec", "gl.xml"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := NewSpecification(*registry, xmlOverloads{})
	if err != nil {
		t.Fatal(err)
	}
	for _, dispatch := range []bool{false, true} {
		pkg := spec.ToPackage(&PackageSpec{API: "gl", Version: Version{2, 1}, TmplDir: "tmpl"})
		pkg.Filter(map[string]bool{}, map[string]bool{"glGetIntegerv": true})
		pkg.ContextDispatch = dispatch
		out := t.TempDir()
		if err := pkg.GeneratePackage(out); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(out, "package.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{"GLint draw, read, viewport[4]", "func (f *Functions) GetFramebufferViewport", "func (f *Functions) GetTargetBindings"} {
			if strings.Contains(string(data), fragment) != dispatch {
				t.Fatalf("dispatch=%v: %s", dispatch, fragment)
			}
		}
	}
}
