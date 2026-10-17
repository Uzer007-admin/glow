// Command glow generates Go OpenGL bindings. See http://github.com/errcw/glow.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

func generate(name string, args []string) {
	flags := flag.NewFlagSet(name, flag.ExitOnError)
	glowBaseDir := determineGlowBaseDir()
	var (
		xmlDir      = flags.String("xml", filepath.Join(glowBaseDir, "xml"), "XML directory")
		tmplDir     = flags.String("tmpl", filepath.Join(glowBaseDir, "tmpl"), "Template directory")
		outDir      = flags.String("out", "gl", "Output directory")
		api         = flags.String("api", "", "API to generate (e.g., gl)")
		ver         = flags.String("version", "", "API version to generate (e.g., 4.1)")
		profile     = flags.String("profile", "", "API profile to generate (e.g., core)")
		addext      = flags.String("addext", "", "If non-empty, a regular expression describing which extensions to include in addition to those supported by the selected profile; takes precedence over explicit removal")
		remext      = flags.String("remext", "", "If non-empty, a regular expression describing which extensions to exclude")
		restrict    = flags.String("restrict", "", "JSON file of symbols to restrict symbol generation")
		lenientInit = flags.Bool("lenientInit", false, "When true missing functions do not fail Init")
		dispatch    = flags.Bool("dispatch", false, "Generate immutable per-context Functions tables in addition to the global API (profiles without debug callbacks)")
	)
	flags.Parse(args)

	version, err := ParseVersion(*ver)
	if err != nil {
		log.Fatalln("error parsing version:", err)
	}

	var addExtRegexp *regexp.Regexp = nil
	if *addext != "" {
		addExtRegexp, err = regexp.Compile(*addext)
		if err != nil {
			log.Fatalln("error parsing extension inclusion regexp:", err)
		}
	}

	var remExtRegexp *regexp.Regexp = nil
	if *remext != "" {
		remExtRegexp, err = regexp.Compile(*remext)
		if err != nil {
			log.Fatalln("error parsing extension exclusion regexp:", err)
		}
	}

	packageSpec := &PackageSpec{
		API:          *api,
		Version:      version,
		Profile:      *profile,
		TmplDir:      *tmplDir,
		AddExtRegexp: addExtRegexp,
		RemExtRegexp: remExtRegexp,
		LenientInit:  *lenientInit,
	}

	specs := parseSpecifications(*xmlDir)
	docs := parseDocumentation(*xmlDir)

	var pkg *Package
	for _, spec := range specs {
		if spec.HasPackage(packageSpec) {
			pkg = spec.ToPackage(packageSpec)
			pkg.ContextDispatch = *dispatch
			docs.AddDocs(pkg)
			if len(*restrict) > 0 {
				if err := performRestriction(pkg, *restrict); err != nil {
					log.Fatalln("error applying symbol restriction:", err)
				}
			}
			if err := pkg.GeneratePackage(*outDir); err != nil {
				log.Fatalln("error generating package:", err)
			}
			if err := copyIncludes(filepath.Join(*xmlDir, "include"), *outDir); err != nil {
				log.Fatalln("error copying includes:", err)
			}
			if err := writeHeaderImports(*outDir, pkg.Name); err != nil {
				log.Fatalln("error writing vendor imports:", err)
			}
			break
		}
	}
	if pkg == nil {
		log.Fatalln("unable to generate package:", packageSpec)
	}
	log.Println("generated package in", *outDir)
}

// Attempt to determine the base directory of go-gl/glow. This only works in case of non-module-aware
// cases and acts as a backwards compatible way.
//
// In a module-only case, this function returns the current working directory.
func determineGlowBaseDir() string {
	glowBaseDir, err := importPathToDir("github.com/Uzer007-admin/glow")
	if err != nil {
		glowBaseDir, err = os.Getwd()
	}
	if err != nil {
		return "."
	}
	return glowBaseDir
}

// Converts a slice string into a simple lookup map.
func lookupMap(s []string) map[string]bool {
	lookup := make(map[string]bool, len(s))
	for _, str := range s {
		lookup[str] = true
	}
	return lookup
}

type jsonRestriction struct {
	Enums     []string `json:"enums"`
	Functions []string `json:"functions"`
}

// Reads the given JSON file path into jsonRestriction and filters the package
// accordingly.
func performRestriction(pkg *Package, jsonPath string) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", jsonPath, err)
	}
	var r *jsonRestriction
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&r); err != nil {
		return fmt.Errorf("parse %s: %w", jsonPath, err)
	}
	if r == nil {
		return fmt.Errorf("%s: expected a restriction object", jsonPath)
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%s: unexpected content after restriction object", jsonPath)
	}
	// Validate before mutating the package so an invalid restriction cannot
	// silently create incomplete bindings. Sort diagnostics for reproducibility.
	var invalid []string
	seen := make(map[string]bool)
	for _, name := range r.Enums {
		if pkg.Enums[name] == nil {
			invalid = append(invalid, "unknown enum "+name)
		} else if seen[name] {
			invalid = append(invalid, "duplicate enum "+name)
		}
		seen[name] = true
	}
	clear(seen)
	for _, name := range r.Functions {
		if pkg.Functions[name] == nil {
			invalid = append(invalid, "unknown function "+name)
		} else if seen[name] {
			invalid = append(invalid, "duplicate function "+name)
		}
		seen[name] = true
	}
	if len(invalid) > 0 {
		sort.Strings(invalid)
		return fmt.Errorf("%s: %s", jsonPath, strings.Join(invalid, "; "))
	}
	// An omitted list keeps that category; an explicitly empty list removes it.
	// Keep Package.Filter's historical zero-length map behavior for its callers.
	if r.Enums != nil {
		keep := lookupMap(r.Enums)
		for name := range pkg.Enums {
			if !keep[name] {
				delete(pkg.Enums, name)
			}
		}
	}
	if r.Functions != nil {
		keep := lookupMap(r.Functions)
		for name := range pkg.Functions {
			if !keep[name] {
				delete(pkg.Functions, name)
			}
		}
	}
	return nil
}

func parseSpecifications(xmlDir string) []*Specification {
	specDir := filepath.Join(xmlDir, "spec")
	overloadDir := filepath.Join(xmlDir, "overload")
	specFiles, err := ioutil.ReadDir(specDir)
	if err != nil {
		log.Fatalln("error reading spec file entries:", err)
	}

	specs := make([]*Specification, 0, len(specFiles))
	for _, specFile := range specFiles {
		if !strings.HasSuffix(specFile.Name(), "xml") {
			continue
		}

		registry, err := readSpecFile(filepath.Join(specDir, specFile.Name()))
		if err != nil {
			log.Fatalln("error reading XML spec file: ", specFile.Name(), err)
		}
		overloads, err := readOverloadFile(filepath.Join(overloadDir, specFile.Name()))
		if err != nil {
			log.Fatalln("error reading XML overload file: ", specFile.Name(), err)
		}
		spec, err := NewSpecification(*registry, overloads)
		if err != nil {
			log.Fatalln("error parsing specification:", specFile.Name(), err)
		}
		specs = append(specs, spec)
	}

	return specs
}

func parseDocumentation(xmlDir string) Documentation {
	docDir := filepath.Join(xmlDir, "doc")
	docFiles, err := ioutil.ReadDir(docDir)
	if err != nil {
		log.Fatalln("error reading doc file entries:", err)
	}

	docs := make([]string, 0, len(docFiles))
	for _, docFile := range docFiles {
		docs = append(docs, filepath.Join(docDir, docFile.Name()))
	}

	doc, err := NewDocumentation(docs)
	if err != nil {
		log.Fatalln("error parsing documentation:", err)
	}

	return doc
}

func copyIncludes(srcDir, dstDir string) error {
	files, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		srcName := filepath.Join(srcDir, file.Name())
		dstName := filepath.Join(dstDir, file.Name())
		switch {
		case file.IsDir():
			if err := os.MkdirAll(dstName, 0755); err != nil {
				return err
			}
			err := copyIncludes(srcName, dstName)
			if err != nil {
				return err
			}
			// Every include directory must survive a fresh go mod vendor.
			stub := "//go:build required\n\n// Package dummy preserves native headers in vendored modules.\npackage dummy\n"
			if err := writeFileIfChanged(filepath.Join(dstName, "dummy.go"), []byte(stub), 0644); err != nil {
				return err
			}
		case file.Size() > 0:
			err := copyFile(srcName, dstName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(srcFile, dstFile string) error {
	data, err := os.ReadFile(srcFile)
	if err != nil {
		return err
	}
	return writeFileIfChanged(dstFile, data, 0644)
}

// writeHeaderImports keeps generated header subdirectories in fresh vendored
// consumers. SDK directories cannot be discovered from C preprocessor text.
func writeHeaderImports(directory, packageName string) error {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	moduleRoot := directory
	var modulePath string
	for {
		data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
		if err == nil {
			modulePath = modfile.ModulePath(data)
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(moduleRoot)
		if parent == moduleRoot {
			return nil
		} // Standalone output without a module.
		moduleRoot = parent
	}
	if modulePath == "" {
		return fmt.Errorf("missing module path in %s", moduleRoot)
	}
	relative, err := filepath.Rel(moduleRoot, directory)
	if err != nil {
		return err
	}
	importPath := modulePath
	if relative != "." {
		importPath += "/" + filepath.ToSlash(relative)
	}
	var imports []string
	err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "dummy.go" && filepath.Dir(path) != directory {
			rel, err := filepath.Rel(directory, filepath.Dir(path))
			if err != nil {
				return err
			}
			imports = append(imports, "\t_ "+strconv.Quote(importPath+"/"+filepath.ToSlash(rel)))
		}
		return nil
	})
	if err != nil || len(imports) == 0 {
		return err
	}
	text := "//go:build required\n\n// Code generated by glow. DO NOT EDIT.\npackage " + packageName + "\n\nimport (\n" + strings.Join(imports, "\n") + "\n)\n"
	return writeFileIfChanged(filepath.Join(directory, "build_cgo_hack.go"), []byte(text), 0644)
}

// PackageSpec describes a package to be generated.
type PackageSpec struct {
	API          string
	Version      Version
	Profile      string // If "all" overrides the version spec
	TmplDir      string
	AddExtRegexp *regexp.Regexp
	RemExtRegexp *regexp.Regexp
	LenientInit  bool
}

func printUsage(name string) {
	fmt.Printf("Usage: %s command [arguments]\n", name)
	fmt.Println("Commands:")
	fmt.Println("  download  Downloads specification and documentation XML files")
	fmt.Println("  generate  Generates bindings")
	fmt.Printf("Use %s <command> -help for a detailed command description\n", name)
}

func main() {
	name := os.Args[0]
	args := os.Args[1:]

	if len(args) < 1 {
		printUsage(name)
		os.Exit(-1)
	}

	command := args[0]
	switch command {
	case "download":
		download("download", args[1:])
	case "generate":
		generate("generate", args[1:])
	default:
		fmt.Printf("Unknown command: '%s'\n", command)
		printUsage(name)
		os.Exit(-1)
	}
}
