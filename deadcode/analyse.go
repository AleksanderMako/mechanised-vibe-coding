package deadcode

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// testRootPrefixes are the function name prefixes the go test driver calls.
var testRootPrefixes = []string{"Test", "Benchmark", "Fuzz", "Example"}

// sourceFunc is one function declaration of a loaded package together with its SSA form.
type sourceFunc struct {
	function    *ssa.Function
	declaration *ast.FuncDecl
	packagePath string
}

// analyse loads the packages under root, computes reachability from the roots and lists the unreachable functions.
func analyse(ctx context.Context, root string, opts Options) ([]Func, error) {
	absoluteRoot, err := checkRoot(root)
	if err != nil {
		return nil, err
	}
	patterns := opts.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	fileSet := token.NewFileSet()
	config := &packages.Config{
		Context: ctx,
		// Dependencies need syntax too, so calls back into our code from them are seen.
		Mode:  packages.LoadAllSyntax | packages.NeedModule,
		Dir:   absoluteRoot,
		Tests: opts.Tests,
		Fset:  fileSet,
	}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, fmt.Errorf("deadcode: load packages: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := firstPackageError(loaded); err != nil {
		return nil, err
	}

	program, ssaPackages := ssautil.AllPackages(loaded, ssa.InstantiateGenerics)
	sources := collectSourceFuncs(program, loaded)
	interfacesByPackage := collectInterfaces(loaded)
	program.Build()

	roots := findRoots(loaded, ssaPackages, opts.Tests)
	if len(roots) == 0 {
		return nil, fmt.Errorf("deadcode: %s: %w", root, ErrNoRoots)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result := rta.Analyze(roots, false)
	// Test variants hold separate copies of one declaration, so liveness is compared by position.
	reachablePositions := map[token.Position]bool{}
	for function := range result.Reachable {
		// An instantiation of a generic function keeps the generic declaration alive.
		origin := function
		if function.Origin() != nil {
			origin = function.Origin()
		}
		if origin.Pos().IsValid() {
			reachablePositions[fileSet.Position(origin.Pos())] = true
		}
	}

	resolvedRoot := resolveSymlinks(absoluteRoot)
	dead := []Func{}
	reported := map[token.Position]bool{}
	for _, source := range sources {
		position := fileSet.Position(source.function.Pos())
		if reachablePositions[position] || reported[position] {
			continue
		}
		reported[position] = true
		// Like cmd/deadcode, the first unreachable copy decides, and marker methods are hidden.
		if isMarkerMethod(source, interfacesByPackage[source.function.Pkg.Pkg]) {
			continue
		}
		declarationPosition := fileSet.Position(source.declaration.Pos())
		dead = append(dead, Func{
			Package: source.packagePath,
			Name:    displayName(source.function),
			File:    relativeFile(resolvedRoot, declarationPosition.Filename),
			Line:    declarationPosition.Line,
		})
	}
	sortFuncs(dead)
	return dead, nil
}

// checkRoot returns root as an absolute path after making sure it holds a go.mod.
func checkRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("deadcode: root is empty")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("deadcode: resolve root: %w", err)
	}
	info, err := os.Stat(filepath.Join(absoluteRoot, "go.mod"))
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("deadcode: %s does not contain go.mod", root)
	}
	return absoluteRoot, nil
}

// firstPackageError returns the first load or type error of the packages or their dependencies.
func firstPackageError(loaded []*packages.Package) error {
	var first error
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		if first == nil && len(pkg.Errors) > 0 {
			first = fmt.Errorf("deadcode: package %s: %v", pkg.ID, pkg.Errors[0])
		}
	})
	if first == nil && len(loaded) == 0 {
		first = fmt.Errorf("deadcode: no packages matched")
	}
	return first
}

// collectSourceFuncs lists the named functions and methods declared in the loaded packages.
// It runs before program.Build so it can still use the syntax trees.
func collectSourceFuncs(program *ssa.Program, loaded []*packages.Package) []sourceFunc {
	sources := []sourceFunc{}
	for _, pkg := range inVisitOrder(loaded) {
		// The generated test main lives in the build cache and holds no code of ours.
		if strings.HasSuffix(pkg.ID, ".test") {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, declaration := range file.Decls {
				funcDecl, isFunc := declaration.(*ast.FuncDecl)
				// init and blank functions stay in: cmd/deadcode lists them when nothing reaches them.
				if !isFunc {
					continue
				}
				object, isFuncObject := pkg.TypesInfo.Defs[funcDecl.Name].(*types.Func)
				if !isFuncObject {
					continue
				}
				function := program.FuncValue(object)
				if function == nil || function.Synthetic != "" || !function.Pos().IsValid() {
					continue
				}
				sources = append(sources, sourceFunc{function: function, declaration: funcDecl, packagePath: pkg.PkgPath})
			}
		}
	}
	return sources
}

// inVisitOrder returns the loaded packages in the dependency-first order cmd/deadcode walks them.
// The order decides which test variant copy of a declaration is judged, which matters for marker methods.
func inVisitOrder(loaded []*packages.Package) []*packages.Package {
	isLoaded := map[*packages.Package]bool{}
	for _, pkg := range loaded {
		isLoaded[pkg] = true
	}
	ordered := []*packages.Package{}
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		if isLoaded[pkg] {
			ordered = append(ordered, pkg)
		}
	})
	return ordered
}

// collectInterfaces lists, per loaded package, the interface types declared at package level.
func collectInterfaces(loaded []*packages.Package) map[*types.Package][]*types.Interface {
	interfacesByPackage := map[*types.Package][]*types.Interface{}
	for _, pkg := range loaded {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			typeName, isTypeName := scope.Lookup(name).(*types.TypeName)
			if isTypeName && types.IsInterface(typeName.Type()) {
				interfacesByPackage[pkg.Types] = append(interfacesByPackage[pkg.Types], typeName.Type().Underlying().(*types.Interface))
			}
		}
	}
	return interfacesByPackage
}

// isMarkerMethod reports whether source is an unexported, empty, parameterless method that satisfies
// an interface of its own package, like protoc's isOneof_Field; cmd/deadcode never reports those.
func isMarkerMethod(source sourceFunc, interfaces []*types.Interface) bool {
	signature := source.function.Signature
	if signature.Recv() == nil || ast.IsExported(source.function.Name()) || signature.Params() != nil || signature.Results() != nil {
		return false
	}
	body := source.declaration.Body
	if body == nil || len(body.List) > 0 {
		return false
	}
	for _, iface := range interfaces {
		if types.Implements(signature.Recv().Type(), iface) {
			return true
		}
	}
	return false
}

// findRoots returns main and init of every main package (test mains included) and, when tests count, the test functions.
// Other packages' initializers are reached through the init of whoever imports them, as in cmd/deadcode.
func findRoots(loaded []*packages.Package, ssaPackages []*ssa.Package, includeTests bool) []*ssa.Function {
	roots := []*ssa.Function{}
	for index, pkg := range loaded {
		ssaPackage := ssaPackages[index]
		if ssaPackage == nil {
			continue
		}
		if ssaPackage.Pkg.Name() == "main" {
			if mainFunction := ssaPackage.Func("main"); mainFunction != nil {
				roots = append(roots, mainFunction)
				if initializer := ssaPackage.Func("init"); initializer != nil {
					roots = append(roots, initializer)
				}
			}
		}
		if !includeTests || !isTestVariant(pkg) {
			continue
		}
		for _, member := range ssaPackage.Members {
			function, isFunction := member.(*ssa.Function)
			if isFunction && isTestRoot(function, ssaPackage.Prog.Fset) {
				roots = append(roots, function)
			}
		}
	}
	return roots
}

// isTestVariant reports whether pkg was compiled for a test binary, such as "p [p.test]".
func isTestVariant(pkg *packages.Package) bool {
	return strings.HasSuffix(pkg.ID, ".test]")
}

// isTestRoot reports whether function is one the go test driver calls directly.
func isTestRoot(function *ssa.Function, fileSet *token.FileSet) bool {
	if !function.Pos().IsValid() || !strings.HasSuffix(fileSet.Position(function.Pos()).Filename, "_test.go") {
		return false
	}
	for _, prefix := range testRootPrefixes {
		if strings.HasPrefix(function.Name(), prefix) {
			return true
		}
	}
	return false
}

// displayName returns "Name" for a function and "Type.Method" for a method, without pointer or type arguments.
// An init function keeps its SSA name such as "init#1", the way cmd/deadcode prints it.
func displayName(function *ssa.Function) string {
	object, isFuncObject := function.Object().(*types.Func)
	if !isFuncObject {
		return function.Name()
	}
	receiver := object.Signature().Recv()
	if receiver == nil {
		return function.Name()
	}
	receiverType := receiver.Type()
	if pointer, isPointer := receiverType.(*types.Pointer); isPointer {
		receiverType = pointer.Elem()
	}
	if named, isNamed := types.Unalias(receiverType).(*types.Named); isNamed {
		return named.Obj().Name() + "." + object.Name()
	}
	return object.Name()
}

// relativeFile returns filename relative to root with forward slashes.
func relativeFile(resolvedRoot, filename string) string {
	relative, err := filepath.Rel(resolvedRoot, resolveSymlinks(filename))
	if err != nil {
		return filepath.ToSlash(filename)
	}
	return filepath.ToSlash(relative)
}

// resolveSymlinks follows symlinks such as macOS /var to /private/var, so paths compare equal.
func resolveSymlinks(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}
