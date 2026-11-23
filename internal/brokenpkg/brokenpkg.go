// Package brokenpkg finds loaded packages with errors and the packages that depend on them,
// so symbols, refactor and deadcode share one definition of "broken" and "depends on".
package brokenpkg

import (
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// IsBroken reports whether pkg has any list, parse or type error.
func IsBroken(pkg *packages.Package) bool {
	return len(pkg.Errors) > 0
}

// InVisitOrder returns every package reachable from roots, dependencies first, in packages.Visit order.
func InVisitOrder(roots []*packages.Package) []*packages.Package {
	var ordered []*packages.Package
	packages.Visit(roots, nil, func(pkg *packages.Package) {
		ordered = append(ordered, pkg)
	})
	return ordered
}

// IndexByID maps the ID of every package reachable from roots to the package.
func IndexByID(roots []*packages.Package) map[string]*packages.Package {
	byID := map[string]*packages.Package{}
	for _, pkg := range InVisitOrder(roots) {
		byID[pkg.ID] = pkg
	}
	return byID
}

// externalTest returns "p_test [p.test]" for the test variant "p [p.test]", or nil when there is none.
func externalTest(pkg *packages.Package, byID map[string]*packages.Package) *packages.Package {
	base, variant, isVariant := strings.Cut(pkg.ID, " [")
	if !isVariant || !strings.HasSuffix(variant, ".test]") || strings.HasSuffix(base, "_test") {
		return nil
	}
	return byID[base+"_test ["+variant]
}

// DependsOn reports whether a package with importPath is among the transitive imports of pkg.
// A test variant "p [p.test]" also depends on its external test package and everything that imports.
func DependsOn(pkg *packages.Package, importPath string, byID map[string]*packages.Package) bool {
	seen := map[*packages.Package]bool{pkg: true}
	var search func(current *packages.Package) bool
	search = func(current *packages.Package) bool {
		for _, imported := range current.Imports {
			if seen[imported] {
				continue
			}
			seen[imported] = true
			if imported.PkgPath == importPath || search(imported) {
				return true
			}
		}
		return false
	}
	if search(pkg) {
		return true
	}
	external := externalTest(pkg, byID)
	if external == nil || seen[external] {
		return false
	}
	seen[external] = true
	return external.PkgPath == importPath || search(external)
}

// BrokenDependencies maps each broken package to its own ID and each package that depends on
// a broken one to the ID of such a broken package; ordered must be in packages.Visit order.
func BrokenDependencies(ordered []*packages.Package) map[*packages.Package]string {
	byID := map[string]*packages.Package{}
	for _, pkg := range ordered {
		byID[pkg.ID] = pkg
	}
	brokenVia := map[*packages.Package]string{}
	for _, pkg := range ordered {
		if IsBroken(pkg) {
			brokenVia[pkg] = pkg.ID
		}
	}
	// Repeat until stable because the external test rule points from a dependency to its importer.
	for changed := true; changed; {
		changed = false
		for _, pkg := range ordered {
			if _, known := brokenVia[pkg]; known {
				continue
			}
			if via, found := firstBrokenImport(pkg, brokenVia, byID); found {
				brokenVia[pkg] = via
				changed = true
			}
		}
	}
	return brokenVia
}

// firstBrokenImport returns the broken package ID reached through pkg's imports in path order,
// then through its external test package.
func firstBrokenImport(pkg *packages.Package, brokenVia map[*packages.Package]string, byID map[string]*packages.Package) (string, bool) {
	importPaths := make([]string, 0, len(pkg.Imports))
	for importPath := range pkg.Imports {
		importPaths = append(importPaths, importPath)
	}
	sort.Strings(importPaths)
	for _, importPath := range importPaths {
		if via, found := brokenVia[pkg.Imports[importPath]]; found {
			return via, true
		}
	}
	if external := externalTest(pkg, byID); external != nil {
		via, found := brokenVia[external]
		return via, found
	}
	return "", false
}
