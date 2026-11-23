package refactor

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// movedUnit is one declaration, or one spec lifted out of a group, that moves.
type movedUnit struct {
	pkg         *packages.Package
	file        string   // absolute source file
	node        ast.Node // the FuncDecl, GenDecl or spec whose identifiers move
	removeStart int      // first byte removed from the source, at the start of the doc comment line
	removeEnd   int      // byte after the last removed one, including the line end
	copyEnd     int      // byte after the last copied one, before the line end
	keyword     string   // "var", "const" or "type" when a single spec leaves its group
	keywordAt   int      // offset where the keyword is inserted
	wholeDecl   bool     // the entire top level declaration moves
}

// moveTarget holds the destination of a move.
type moveTarget struct {
	importPath  string
	dir         string
	file        string // absolute destination file
	packageName string
	names       map[string]bool // package level names already declared in the destination
}

// move implements Move.
func move(ctx context.Context, root string, syms []symbols.Symbol, toImportPath, toFile string, opts Options) ([]string, error) {
	if _, err := checkRoot(root); err != nil {
		return nil, err
	}
	if len(syms) == 0 {
		return nil, fmt.Errorf("refactor: no symbols to move")
	}
	sourcePath := syms[0].Package
	for _, sym := range syms {
		if sym.Package != sourcePath {
			return nil, fmt.Errorf("refactor: %s and %s are in different packages", syms[0], sym)
		}
		if strings.Contains(sym.Name, ".") {
			return nil, fmt.Errorf("refactor: %s is not a package level declaration", sym)
		}
	}
	if toImportPath == sourcePath {
		return nil, fmt.Errorf("refactor: %s is already in %s", syms[0], toImportPath)
	}
	if err := checkFileName(toFile); err != nil {
		return nil, err
	}
	work, err := load(ctx, root, opts, []string{sourcePath}, toImportPath)
	if err != nil {
		return nil, err
	}
	target, err := work.moveTarget(toImportPath, toFile)
	if err != nil {
		return nil, err
	}

	// Resolve every symbol and remember the moved package level objects by key.
	moved := map[objectKey]types.Object{}
	var objects []types.Object
	for _, sym := range syms {
		object, err := work.resolve(sym)
		if err != nil {
			return nil, err
		}
		if !isPackageLevel(object) {
			return nil, fmt.Errorf("refactor: %s is not a package level declaration", sym)
		}
		key := work.keyOf(object)
		if _, duplicate := moved[key]; duplicate {
			continue
		}
		moved[key] = object
		objects = append(objects, object)
	}
	sourceName := objects[0].Pkg().Name()

	units, err := work.collectUnits(objects, moved)
	if err != nil {
		return nil, err
	}
	movedRanges := map[string][][2]int{}
	for _, unit := range units {
		movedRanges[unit.file] = append(movedRanges[unit.file], [2]int{unit.removeStart, unit.removeEnd})
	}
	insideMoved := func(pos token.Pos) bool {
		position := work.fileSet.Position(pos)
		for _, span := range movedRanges[position.Filename] {
			if position.Offset >= span[0] && position.Offset < span[1] {
				return true
			}
		}
		return false
	}

	edits := newEditSet(work.fileSet)

	// Walk the moved code: drop destination qualifiers, qualify exported source symbols,
	// collect the imports it needs and the members (fields, methods) it declares.
	members := map[objectKey]types.Object{}
	var leftBehind []string
	var memberUses []types.Object
	needsSourceImport := false
	for _, unit := range units {
		info := unit.pkg.TypesInfo
		ast.Inspect(unit.node, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.SelectorExpr:
				if qualifier, isIdent := typed.X.(*ast.Ident); isIdent {
					if pkgName, isPkgName := info.Uses[qualifier].(*types.PkgName); isPkgName && pkgName.Imported().Path() == toImportPath {
						edits.replace(qualifier.Pos(), typed.Sel.Pos(), "")
					}
				}
			case *ast.Ident:
				if defined := info.Defs[typed]; defined != nil && isMember(defined) {
					members[work.keyOf(defined)] = defined
				}
				switch used := info.Uses[typed].(type) {
				case nil:
				case *types.PkgName:
					if used.Imported().Path() != toImportPath {
						spec := importSpec{path: used.Imported().Path()}
						if used.Name() != used.Imported().Name() {
							spec.name = used.Name()
						}
						edits.addImport(target.file, spec)
					}
				default:
					// Unexported fields and methods of types that stay behind are unreachable from the destination too.
					if isMember(used) && !used.Exported() && used.Pkg() != nil && used.Pkg().Path() == sourcePath {
						memberUses = append(memberUses, used)
					}
					if !isPackageLevel(used) || used.Pkg().Path() != sourcePath {
						break
					}
					if _, isMoved := moved[work.keyOf(used)]; isMoved {
						break
					}
					if !used.Exported() {
						leftBehind = append(leftBehind, used.Name())
						break
					}
					// The symbol stays behind, so the moved code reaches it through the source package.
					edits.replace(typed.Pos(), typed.End(), sourceName+"."+used.Name())
					needsSourceImport = true
				}
			}
			return true
		})
	}
	for _, member := range memberUses {
		if _, declaredInMovedCode := members[work.keyOf(member)]; !declaredInMovedCode {
			leftBehind = append(leftBehind, member.Name())
		}
	}
	if len(leftBehind) > 0 {
		slices.Sort(leftBehind)
		return nil, fmt.Errorf("refactor: %w: %s", ErrLeftBehind, strings.Join(slices.Compact(leftBehind), ", "))
	}
	if needsSourceImport {
		spec := importSpec{path: sourcePath}
		if sourceName != path.Base(sourcePath) {
			spec.name = sourceName
		}
		edits.addImport(target.file, spec)
	}

	// Find the references to moved objects outside the moved code, and which of them leave the destination.
	type outsideRef struct {
		pkg   *packages.Package
		ident *ast.Ident
	}
	var outside []outsideRef
	usedOutside := map[objectKey]bool{}
	// Comparing names first avoids computing positions for every identifier of the module.
	watchedNames := map[string]bool{}
	for _, object := range moved {
		watchedNames[object.Name()] = true
	}
	for _, member := range members {
		watchedNames[member.Name()] = true
	}
	for _, pkg := range work.packages {
		for ident, used := range pkg.TypesInfo.Uses {
			if !watchedNames[ident.Name] {
				continue
			}
			key := work.keyOf(used)
			_, isMoved := moved[key]
			_, isMember := members[key]
			if (!isMoved && !isMember) || insideMoved(ident.Pos()) {
				continue
			}
			if isMoved {
				outside = append(outside, outsideRef{pkg: pkg, ident: ident})
			}
			if pkg.PkgPath != toImportPath {
				usedOutside[key] = true
			}
		}
	}

	// Unexported objects still used from outside the destination must become exported.
	finalNames := map[objectKey]string{}
	for key, object := range moved {
		finalNames[key] = object.Name()
		if !object.Exported() && usedOutside[key] {
			finalNames[key] = exportedForm(object.Name())
		}
	}
	taken := map[string]objectKey{}
	for key, name := range finalNames {
		if target.names[name] {
			return nil, fmt.Errorf("refactor: %s is already declared in %s: %w", name, toImportPath, ErrConflict)
		}
		if other, clash := taken[name]; clash && other != key {
			return nil, fmt.Errorf("refactor: two moved declarations would both be named %s: %w", name, ErrConflict)
		}
		taken[name] = key
	}
	for key, member := range members {
		if member.Exported() || !usedOutside[key] {
			continue
		}
		newName := exportedForm(member.Name())
		if err := work.checkMemberExport(member, newName, objects); err != nil {
			return nil, err
		}
		for _, use := range work.identsOf(map[objectKey]bool{key: true}, member.Name()) {
			edits.replace(use.ident.Pos(), use.ident.End(), newName)
		}
		if declared, found := work.findDeclaration(member); found {
			for _, doc := range docOf(declared.path) {
				docFirstWordEdit(edits, doc, member.Name(), newName)
			}
		}
	}

	// Rename moved objects inside the moved code, declarations included.
	for key, object := range moved {
		finalName := finalNames[key]
		if finalName == object.Name() {
			continue
		}
		for _, use := range work.identsOf(map[objectKey]bool{key: true}, object.Name()) {
			if insideMoved(use.ident.Pos()) {
				edits.replace(use.ident.Pos(), use.ident.End(), finalName)
			}
		}
		if declared, found := work.findDeclaration(object); found {
			for _, doc := range docOf(declared.path) {
				docFirstWordEdit(edits, doc, object.Name(), finalName)
			}
		}
	}

	// Rewrite references outside the moved code to their place after the move.
	parentsByFile := map[*ast.File]*parentMaps{}
	for _, ref := range outside {
		file := work.fileOf(ref.pkg, ref.ident.Pos())
		if file == nil {
			continue
		}
		parents, cached := parentsByFile[file]
		if !cached {
			parents = buildParentMaps(file)
			parentsByFile[file] = parents
		}
		finalName := finalNames[work.keyOf(ref.pkg.TypesInfo.Uses[ref.ident])]
		start := ref.ident.Pos()
		if selector := parents.selectorOf[ref.ident]; selector != nil {
			if qualifier, isIdent := selector.X.(*ast.Ident); isIdent {
				if _, isPkgName := ref.pkg.TypesInfo.Uses[qualifier].(*types.PkgName); isPkgName {
					start = selector.Pos()
				}
			}
		}
		replacement := finalName
		if ref.pkg.PkgPath != toImportPath {
			replacement = qualifierFor(edits, ref.pkg, file, toImportPath, target.packageName) + finalName
		}
		edits.replace(start, ref.ident.End(), replacement)
	}

	changes, err := work.moveChanges(edits, units, target)
	if err != nil {
		return nil, err
	}
	return commit(ctx, work.root, changes, opts)
}

// checkFileName accepts a plain .go file name inside the destination directory.
func checkFileName(toFile string) error {
	if toFile == "" || filepath.IsAbs(toFile) || filepath.Base(toFile) != toFile || !strings.HasSuffix(toFile, ".go") ||
		toFile == ".go" {
		return fmt.Errorf("refactor: destination file %q must be a .go file name in the package directory", toFile)
	}
	return nil
}

// isMember reports whether object is a struct field or a method.
func isMember(object types.Object) bool {
	switch typed := object.(type) {
	case *types.Var:
		return typed.IsField()
	case *types.Func:
		return typed.Type().(*types.Signature).Recv() != nil
	}
	return false
}

// moveTarget resolves the destination directory, file, package name and declared names.
func (w *workspace) moveTarget(importPath, fileName string) (*moveTarget, error) {
	if importPath != w.modulePath && !strings.HasPrefix(importPath, w.modulePath+"/") {
		return nil, fmt.Errorf("refactor: %s is not inside module %s", importPath, w.modulePath)
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(importPath, w.modulePath), "/")
	dir := filepath.Join(w.root, filepath.FromSlash(relative))
	target := &moveTarget{importPath: importPath, dir: dir, file: filepath.Join(dir, fileName), names: map[string]bool{}}

	loaded := w.packagesWithPath(importPath)
	for _, pkg := range loaded {
		if target.packageName == "" || pkg.ID == pkg.PkgPath {
			target.packageName = pkg.Name
		}
		for _, name := range pkg.Types.Scope().Names() {
			target.names[name] = true
		}
	}
	if len(loaded) == 0 {
		// The destination is not loaded: read its files directly, or name a new package after its directory.
		if err := readUnloadedPackage(target); err != nil {
			return nil, err
		}
	}
	if target.packageName == "" {
		target.packageName = path.Base(importPath)
		if !token.IsIdentifier(target.packageName) {
			return nil, fmt.Errorf("refactor: %s cannot be a package name", target.packageName)
		}
	}
	return target, nil
}

// readUnloadedPackage fills the package name and declared names from the .go files in target.dir.
func readUnloadedPackage(target *moveTarget) error {
	entries, err := os.ReadDir(target.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("refactor: read %s: %w", target.dir, err)
	}
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, filepath.Join(target.dir, entry.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("refactor: parse destination: %w", err)
		}
		// External test packages are a different package and do not name the destination.
		if strings.HasSuffix(parsed.Name.Name, "_test") {
			continue
		}
		if target.packageName == "" || !strings.HasSuffix(entry.Name(), "_test.go") {
			target.packageName = parsed.Name.Name
		}
		for _, decl := range parsed.Decls {
			for _, name := range declaredNames(decl) {
				target.names[name] = true
			}
		}
	}
	return nil
}

// declaredNames lists the package level names a declaration introduces.
func declaredNames(decl ast.Decl) []string {
	var names []string
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if typed.Recv == nil {
			names = append(names, typed.Name.Name)
		}
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			switch specTyped := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, specTyped.Name.Name)
			case *ast.ValueSpec:
				for _, name := range specTyped.Names {
					names = append(names, name.Name)
				}
			}
		}
	}
	return names
}

// checkMemberExport rejects exporting a member when its type already has a member with the exported name.
func (w *workspace) checkMemberExport(member types.Object, newName string, movedObjects []types.Object) error {
	var owner types.Type
	if method, isFunc := member.(*types.Func); isFunc {
		owner = method.Type().(*types.Signature).Recv().Type()
	} else {
		memberKey := w.keyOf(member)
		for _, object := range movedObjects {
			structType, isStruct := object.Type().Underlying().(*types.Struct)
			if _, isTypeName := object.(*types.TypeName); !isTypeName || !isStruct {
				continue
			}
			for field := range structType.Fields() {
				if w.keyOf(field) == memberKey {
					owner = object.Type()
				}
			}
		}
	}
	if owner == nil {
		return nil
	}
	if existing, _, _ := types.LookupFieldOrMethod(owner, true, member.Pkg(), newName); existing != nil {
		return fmt.Errorf("refactor: %s already has a member named %s: %w", owner, newName, ErrConflict)
	}
	return nil
}

// collectUnits finds the declarations of the moved objects plus the methods of moved types.
func (w *workspace) collectUnits(objects []types.Object, moved map[objectKey]types.Object) ([]*movedUnit, error) {
	type groupKey struct {
		file   string
		offset int
	}
	type groupInfo struct {
		pkg   *packages.Package
		decl  *ast.GenDecl
		specs map[ast.Spec]bool
	}
	groups := map[groupKey]*groupInfo{}
	var groupOrder []groupKey
	var units []*movedUnit
	seenUnits := map[groupKey]bool{}

	for _, object := range objects {
		declared, found := w.findDeclaration(object)
		if !found {
			return nil, fmt.Errorf("refactor: %s.%s is not declared in the loaded packages", object.Pkg().Path(), object.Name())
		}
		for index, node := range declared.path {
			if funcDecl, isFunc := node.(*ast.FuncDecl); isFunc {
				unit := w.newUnit(declared.pkg, funcDecl, funcDecl.Doc)
				unit.wholeDecl = true
				key := groupKey{unit.file, unit.removeStart}
				if !seenUnits[key] {
					seenUnits[key] = true
					units = append(units, unit)
				}
				break
			}
			spec, isSpec := node.(ast.Spec)
			if !isSpec || index+1 >= len(declared.path) {
				continue
			}
			genDecl := declared.path[index+1].(*ast.GenDecl)
			key := groupKey{fileNameOf(w.fileSet, genDecl.Pos()), offsetOf(w.fileSet, genDecl.Pos())}
			group, known := groups[key]
			if !known {
				group = &groupInfo{pkg: declared.pkg, decl: genDecl, specs: map[ast.Spec]bool{}}
				groups[key] = group
				groupOrder = append(groupOrder, key)
			}
			group.specs[spec] = true
			break
		}
	}

	for _, key := range groupOrder {
		group := groups[key]
		if len(group.specs) == len(group.decl.Specs) && w.allNamesMoved(group.pkg, group.decl.Specs, moved) {
			unit := w.newUnit(group.pkg, group.decl, group.decl.Doc)
			unit.wholeDecl = true
			units = append(units, unit)
			continue
		}
		for _, spec := range group.decl.Specs {
			if !group.specs[spec] {
				continue
			}
			if err := w.checkLiftableSpec(group.pkg, group.decl, spec, moved); err != nil {
				return nil, err
			}
			unit := w.newUnit(group.pkg, spec, specDoc(spec))
			unit.keyword = group.decl.Tok.String()
			unit.keywordAt = offsetOf(w.fileSet, spec.Pos())
			units = append(units, unit)
		}
	}

	// Methods must live in the package of their type, so they move with it.
	for _, object := range objects {
		if _, isTypeName := object.(*types.TypeName); !isTypeName {
			continue
		}
		for _, unit := range w.methodUnits(object) {
			key := groupKey{unit.file, unit.removeStart}
			if !seenUnits[key] {
				seenUnits[key] = true
				units = append(units, unit)
			}
		}
	}
	slices.SortFunc(units, func(a, b *movedUnit) int {
		if a.file != b.file {
			return strings.Compare(a.file, b.file)
		}
		return a.removeStart - b.removeStart
	})
	return units, nil
}

// allNamesMoved reports whether every name declared by specs is among the moved objects.
func (w *workspace) allNamesMoved(pkg *packages.Package, specs []ast.Spec, moved map[objectKey]types.Object) bool {
	for _, spec := range specs {
		for _, ident := range specIdents(spec) {
			if ident.Name == "_" {
				continue
			}
			object := pkg.TypesInfo.Defs[ident]
			if object == nil {
				return false
			}
			if _, isMoved := moved[w.keyOf(object)]; !isMoved {
				return false
			}
		}
	}
	return true
}

// checkLiftableSpec rejects lifting a spec out of its group when that would change its meaning.
func (w *workspace) checkLiftableSpec(pkg *packages.Package, decl *ast.GenDecl, spec ast.Spec, moved map[objectKey]types.Object) error {
	if !w.allNamesMoved(pkg, []ast.Spec{spec}, moved) {
		return fmt.Errorf("refactor: %s declares names that do not move: %w", strings.Join(identNames(specIdents(spec)), ", "), ErrUnsupportedUse)
	}
	if decl.Tok != token.CONST {
		return nil
	}
	// Constants in a group may repeat the previous expression or use iota, which depend on the group.
	for _, other := range decl.Specs {
		valueSpec := other.(*ast.ValueSpec)
		if len(valueSpec.Values) == 0 {
			return fmt.Errorf("refactor: constant %s depends on its group: %w", strings.Join(identNames(specIdents(spec)), ", "), ErrUnsupportedUse)
		}
	}
	usesIota := false
	ast.Inspect(spec, func(node ast.Node) bool {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			if used := pkg.TypesInfo.Uses[ident]; used != nil && used.Parent() == types.Universe && used.Name() == "iota" {
				usesIota = true
			}
		}
		return true
	})
	if usesIota {
		return fmt.Errorf("refactor: constant %s uses iota of its group: %w", strings.Join(identNames(specIdents(spec)), ", "), ErrUnsupportedUse)
	}
	return nil
}

// methodUnits returns a unit for every method declared on typeName in its package.
func (w *workspace) methodUnits(typeName types.Object) []*movedUnit {
	typeKey := w.keyOf(typeName)
	var units []*movedUnit
	for _, pkg := range w.packagesWithPath(typeName.Pkg().Path()) {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				funcDecl, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
					continue
				}
				base := receiverBaseIdent(funcDecl.Recv.List[0].Type)
				if base == nil {
					continue
				}
				if used := pkg.TypesInfo.Uses[base]; used != nil && w.keyOf(used) == typeKey {
					unit := w.newUnit(pkg, funcDecl, funcDecl.Doc)
					unit.wholeDecl = true
					units = append(units, unit)
				}
			}
		}
	}
	return units
}

// receiverBaseIdent returns the type name identifier of a receiver type expression.
func receiverBaseIdent(expr ast.Expr) *ast.Ident {
	for {
		switch typed := expr.(type) {
		case *ast.Ident:
			return typed
		case *ast.StarExpr:
			expr = typed.X
		case *ast.ParenExpr:
			expr = typed.X
		case *ast.IndexExpr:
			expr = typed.X
		case *ast.IndexListExpr:
			expr = typed.X
		default:
			return nil
		}
	}
}

// newUnit computes the byte ranges of node and its doc comment in its file.
func (w *workspace) newUnit(pkg *packages.Package, node ast.Node, doc *ast.CommentGroup) *movedUnit {
	start := node.Pos()
	if doc != nil {
		start = doc.Pos()
	}
	file := fileNameOf(w.fileSet, start)
	unit := &movedUnit{pkg: pkg, file: file, node: node}
	unit.removeStart = offsetOf(w.fileSet, start)
	unit.copyEnd = offsetOf(w.fileSet, node.End())
	unit.removeEnd = unit.copyEnd
	return unit
}

// widenToLines extends a unit over its indentation and the rest of its last line, using the file bytes.
func widenToLines(unit *movedUnit, content []byte) {
	start := unit.removeStart
	for start > 0 && (content[start-1] == ' ' || content[start-1] == '\t') {
		start--
	}
	if start == 0 || content[start-1] == '\n' {
		unit.removeStart = start
	}
	end := unit.copyEnd
	lineEnd := end
	for lineEnd < len(content) && content[lineEnd] != '\n' {
		lineEnd++
	}
	rest := strings.TrimSpace(string(content[end:lineEnd]))
	// A trailing line comment belongs to the declaration and travels with it.
	if rest == "" || strings.HasPrefix(rest, "//") {
		unit.copyEnd = lineEnd
		unit.removeEnd = lineEnd
		if lineEnd < len(content) {
			unit.removeEnd = lineEnd + 1
		}
	}
}

// specIdents returns the identifiers a spec declares.
func specIdents(spec ast.Spec) []*ast.Ident {
	switch typed := spec.(type) {
	case *ast.TypeSpec:
		return []*ast.Ident{typed.Name}
	case *ast.ValueSpec:
		return typed.Names
	}
	return nil
}

// specDoc returns the doc comment of a spec.
func specDoc(spec ast.Spec) *ast.CommentGroup {
	switch typed := spec.(type) {
	case *ast.TypeSpec:
		return typed.Doc
	case *ast.ValueSpec:
		return typed.Doc
	}
	return nil
}

func identNames(idents []*ast.Ident) []string {
	var names []string
	for _, ident := range idents {
		names = append(names, ident.Name)
	}
	return names
}

// moveChanges applies the edits, removes the moved code from the source files,
// deletes emptied files and appends the moved code to the destination file.
func (w *workspace) moveChanges(edits *editSet, units []*movedUnit, target *moveTarget) ([]*fileChange, error) {
	unitsByFile := map[string][]*movedUnit{}
	for _, unit := range units {
		unitsByFile[unit.file] = append(unitsByFile[unit.file], unit)
	}
	files := edits.files()
	for file := range unitsByFile {
		files = append(files, file)
	}
	files = append(files, target.file)
	slices.Sort(files)
	files = slices.Compact(files)

	// The moved text is cut from the original bytes with the edits inside it applied.
	var movedTexts []string
	originals := map[string]*fileChange{}
	for _, file := range files {
		if file == target.file {
			if _, err := os.Stat(file); os.IsNotExist(err) {
				originals[file] = &fileChange{path: file, mode: 0o644}
				continue
			}
		}
		change, err := readChange(file)
		if err != nil {
			return nil, err
		}
		originals[file] = change
	}
	for _, file := range files {
		for _, unit := range unitsByFile[file] {
			content := originals[file].original
			widenToLines(unit, content)
			unitEdits := editsWithin(edits.edits[file], unit.removeStart, unit.copyEnd)
			if unit.keyword != "" {
				// A spec lifted out of a group needs its own keyword in front of it.
				at := unit.keywordAt - unit.removeStart
				unitEdits = append(unitEdits, textEdit{start: at, end: at, text: unit.keyword + " "})
			}
			text, err := applyEdits(file, content[unit.removeStart:unit.copyEnd], unitEdits)
			if err != nil {
				return nil, err
			}
			movedTexts = append(movedTexts, strings.TrimSpace(string(text)))
		}
	}

	var changes []*fileChange
	for _, file := range files {
		change := originals[file]
		fileUnits := unitsByFile[file]
		var fileEdits []textEdit
		for _, edit := range edits.edits[file] {
			if !insideAnyUnit(edit, fileUnits) {
				fileEdits = append(fileEdits, edit)
			}
		}
		for _, unit := range fileUnits {
			fileEdits = append(fileEdits, textEdit{start: unit.removeStart, end: unit.removeEnd})
		}
		content, err := applyEdits(file, change.original, fileEdits)
		if err != nil {
			return nil, err
		}
		if file == target.file {
			if !change.existed {
				content = []byte("package " + target.packageName + "\n")
			}
			content = append(content, []byte("\n"+strings.Join(movedTexts, "\n\n")+"\n")...)
		}
		content, err = addImports(file, content, edits.imports[file])
		if err != nil {
			return nil, err
		}
		change.content = content
		if len(fileUnits) > 0 && w.leftEmpty(file, fileUnits) {
			change.content = nil
		}
		changes = append(changes, change)
	}
	return changes, nil
}

// insideAnyUnit reports whether edit lies within the removed range of one of units.
func insideAnyUnit(edit textEdit, units []*movedUnit) bool {
	for _, unit := range units {
		if edit.start >= unit.removeStart && edit.end <= unit.removeEnd {
			return true
		}
	}
	return false
}

// leftEmpty reports whether every declaration other than imports in file moves away.
func (w *workspace) leftEmpty(file string, units []*movedUnit) bool {
	removed := map[int]bool{}
	for _, unit := range units {
		if unit.wholeDecl {
			removed[offsetOf(w.fileSet, unit.node.Pos())] = true
		}
	}
	for _, pkg := range w.packages {
		for _, syntax := range pkg.Syntax {
			if fileNameOf(w.fileSet, syntax.Pos()) != file {
				continue
			}
			for _, decl := range syntax.Decls {
				if genDecl, isGen := decl.(*ast.GenDecl); isGen && genDecl.Tok == token.IMPORT {
					continue
				}
				if !removed[offsetOf(w.fileSet, decl.Pos())] {
					return false
				}
			}
			return true
		}
	}
	return false
}
