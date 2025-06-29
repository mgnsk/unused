package main

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"iter"
	"log"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"

	"github.com/gobwas/glob"
	"golang.org/x/tools/go/packages"
)

//go:embed doc.go
var doc string

type globFlags []glob.Glob

func (i *globFlags) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *globFlags) Set(value string) error {
	g, err := glob.Compile(value)
	if err != nil {
		return err
	}
	*i = append(*i, g)
	return nil
}

func (i *globFlags) Match(s string) bool {
	for _, g := range *i {
		if g.Match(s) {
			return true
		}
	}
	return false
}

var (
	includeGenerated = false
	excludeFiles     globFlags
	excludeNames     globFlags
	cpuProfile       = ""
	memProfile       = ""
)

func usage() {
	// Extract the content of the /* ... */ comment in doc.go.
	_, after, _ := strings.Cut(doc, "/*\n")
	doc, _, _ := strings.Cut(after, "*/")
	io.WriteString(flag.CommandLine.Output(), doc+`
Flags:

`)
	flag.PrintDefaults()
}

func main() {
	log.SetPrefix("unused: ")
	log.SetFlags(0) // no time prefix

	flag.Usage = usage
	flag.BoolVar(&includeGenerated, "generated", false, "include unused code in generated Go files")
	flag.Var(&excludeFiles, "exclude-files", "exclude file paths by GLOB")
	flag.Var(&excludeNames, "exclude-names", "exclude object names by GLOB")
	flag.StringVar(&cpuProfile, "cpuprofile", "", "write CPU profile to this file")
	flag.StringVar(&memProfile, "memprofile", "", "write memory profile to this file")
	flag.Parse()

	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	if memProfile != "" {
		f, err := os.Create(memProfile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		defer func() {
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Fatalf("error writing memprofile: %v", err)
			}
		}()
	}

	goMod, err := getModule()
	if err != nil {
		log.Fatalf("could not find go module: %v", err)
	}

	// Note: LoadSyntax loads types for all the initial packages
	// but skips testdata packages.
	// LoadAllSyntax also loads testdata packages but is ~6x slower.
	mode := packages.LoadSyntax | packages.NeedModule
	cfg := &packages.Config{
		Mode:  mode,
		Dir:   goMod.Dir,
		Tests: true,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Fatal(err)
	}
	if len(pkgs) == 0 {
		log.Fatalf("no packages found")
	}
	if packages.PrintErrors(pkgs) > 0 {
		log.Fatalf("packages contain errors")
	}

	result := collect(pkgs, goMod.Path)

	exitCode := 0

	opts := options{
		includeGenerated: includeGenerated,
		excludeFiles:     excludeFiles,
		excludeNames:     excludeNames,
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	for o := range result.getUnused(opts) {
		exitCode = 1

		filename, err := filepath.Rel(wd, o.pos.String())
		if err != nil {
			log.Fatal(err)
		}

		if _, err := fmt.Fprintf(os.Stdout, "%s: unused %s %s\n", filename, o.typ, o.name); err != nil {
			log.Fatal(err)
		}
	}

	os.Exit(exitCode)
}

// object is a generic object in code. We key the maps with this value-based type
// instead of comparing `types` pointers to avoid duplicate but different
// pointer, for example *types.Func in packages where test files use the same package name.
type object struct {
	pkgPath string
	name    string
	typ     string
	pos     token.Position
}

func newObject(typ string, fset *token.FileSet, o types.Object) object {
	return object{
		pkgPath: o.Pkg().Path(),
		name:    o.Name(),
		typ:     typ,
		pos:     fset.Position(o.Pos()),
	}
}

// result is a node definition and usage result.
type result struct {
	unusedSkip    []token.Position    // Positions of unused:skip comments.
	unusedDisable []token.Position    // Positions of unused:disable comments.
	generated     map[string]struct{} // File names of generated files.
	defs          map[object]struct{}
	uses          map[object]struct{}
}

type options struct {
	includeGenerated bool
	excludeFiles     globFlags
	excludeNames     globFlags
}

// getUnused returns unused nodes.
func (r result) getUnused(opts options) iter.Seq[object] {
	sortedDefs := slices.SortedFunc(maps.Keys(r.defs), func(a, b object) int {
		return cmp.Or(
			strings.Compare(a.pos.Filename, b.pos.Filename),
			cmp.Compare(a.pos.Offset, b.pos.Offset),
		)
	})

	return func(yield func(object) bool) {
		for _, o := range sortedDefs {
			if _, ok := r.uses[o]; ok {
				continue
			}

			if strings.Contains(o.pkgPath, "testdata") {
				// Skip objects defined in testdata packages.
				continue
			}

			if opts.excludeFiles.Match(o.pos.Filename) {
				continue
			}

			if opts.excludeNames.Match(o.name) {
				continue
			}

			if !opts.includeGenerated {
				if _, ok := r.generated[o.pos.Filename]; ok {
					continue
				}
			}

			if slices.ContainsFunc(r.unusedSkip, func(pos token.Position) bool {
				// Skip if there is a unused:skip comment in the current or previous line.
				return pos.Filename == o.pos.Filename && (pos.Line == o.pos.Line || pos.Line == o.pos.Line-1)
			}) {
				continue
			}

			if slices.ContainsFunc(r.unusedDisable, func(pos token.Position) bool {
				// Skip if there is a unused:disable comment before the current line.
				return pos.Filename == o.pos.Filename && pos.Line < o.pos.Line
			}) {
				continue
			}

			if !yield(o) {
				return
			}
		}
	}
}

// collect the object result from packages.
func collect(pkgs []*packages.Package, filterModule string) result {
	r := result{
		generated: map[string]struct{}{},
		defs:      map[object]struct{}{},
		uses:      map[object]struct{}{},
	}

	packages.Visit(pkgs, func(pkg *packages.Package) bool {
		if pkg.Module == nil {
			return false
		}

		if filterModule != "" && pkg.Module.Path != filterModule {
			return false
		}

		receiverUsesIdents := map[*ast.Ident]struct{}{}
		assignedToVars := map[*ast.Ident]struct{}{}

		for _, f := range pkg.Syntax {
			for _, group := range f.Comments {
				for _, comment := range group.List {
					if strings.HasPrefix(comment.Text, "//unused:skip") || strings.HasPrefix(comment.Text, "// unused:skip") {
						r.unusedSkip = append(r.unusedSkip, pkg.Fset.Position(comment.Pos()))
					}
					if strings.HasPrefix(comment.Text, "//unused:disable") || strings.HasPrefix(comment.Text, "// unused:disable") {
						r.unusedDisable = append(r.unusedDisable, pkg.Fset.Position(comment.Pos()))
					}
				}
			}

			if ast.IsGenerated(f) {
				r.generated[pkg.Fset.File(f.FileStart).Name()] = struct{}{}
			}

			for n := range ast.Preorder(f) {
				switch n := n.(type) {
				case *ast.FuncDecl:
					if id := getFuncDeclReceiver(pkg.Fset, n); id != nil {
						receiverUsesIdents[id] = struct{}{}
					}

				case *ast.AssignStmt:
					for _, lhs := range n.Lhs {
						if id := extractIdent(lhs); id != nil {
							assignedToVars[id] = struct{}{}
						}
					}
				}
			}
		}

		if pkg.TypesInfo == nil {
			return true
		}

		for _, o := range createObjects(pkg.TypesInfo.Defs, pkg.Fset) {
			r.defs[o] = struct{}{}
		}

		for id, o := range createObjects(pkg.TypesInfo.Uses, pkg.Fset) {
			if _, ok := receiverUsesIdents[id]; ok {
				// Don't count implementing a method on type as a use of the receiver type.
				continue
			}
			if _, ok := assignedToVars[id]; ok {
				// Don't count assigning to variable as a use of that variable.
				continue
			}
			r.uses[o] = struct{}{}
		}

		return true
	}, nil)

	return r
}

func createObjects(src map[*ast.Ident]types.Object, fset *token.FileSet) iter.Seq2[*ast.Ident, object] {
	return func(yield func(*ast.Ident, object) bool) {
		for id, obj := range src {
			switch obj := obj.(type) {
			case *types.TypeName:
				if obj := getType(obj); obj != nil && !yield(id, newObject("type", fset, obj)) {
					return
				}

			case *types.Func:
				if obj := getFunc(obj); obj != nil && !yield(id, newObject("func", fset, obj)) {
					return
				}

			case *types.Const:
				if obj := getConst(obj); obj != nil && !yield(id, newObject("const", fset, obj)) {
					return
				}

			case *types.Var:
				if obj := getVar(obj); obj != nil && !yield(id, newObject("var", fset, obj)) {
					return
				}
			}
		}
	}
}

func getType(obj *types.TypeName) *types.TypeName {
	if obj.Pkg() == nil {
		// Skip built-in types.
		return nil
	}

	if strings.HasPrefix(obj.Name(), "_") {
		// Skip some CGO-related types.
		return nil
	}

	if _, ok := obj.Type().(*types.Named); ok {
		return obj
	}

	return nil
}

func getFunc(obj *types.Func) *types.Func {
	if obj.Signature().Recv() != nil {
		// Skip methods.
		return nil
	}

	if obj.Name() == "main" ||
		obj.Name() == "init" ||
		strings.HasPrefix(obj.Name(), "Test") ||
		strings.HasPrefix(obj.Name(), "_") {
		return nil
	}

	return obj
}

func getConst(obj *types.Const) *types.Const {
	if obj.Pkg() == nil {
		// Skip built-in constants.
		return nil
	}

	if strings.HasPrefix(obj.Name(), "_") {
		return nil
	}

	if obj.Parent() != obj.Pkg().Scope() {
		// Declared in a local scope.
		return nil
	}

	return obj
}

func getVar(obj *types.Var) *types.Var {
	if obj.Pkg() == nil {
		// Skip built-in vars, for forward compatibility (none currently).
		return nil
	}

	if strings.HasPrefix(obj.Name(), "_") {
		return nil
	}

	if obj.Embedded() {
		return nil
	}

	if obj.IsField() {
		return nil
	}

	if obj.Parent() != obj.Pkg().Scope() {
		// Declared in a local scope.
		return nil
	}

	return obj
}

type goModule struct {
	Path string
	Dir  string
}

func getModule() (*goModule, error) {
	out, err := exec.Command("go", "list", "-m", "-json").CombinedOutput()
	if err != nil {
		return nil, err
	}
	mod := &goModule{}
	if err := json.Unmarshal(out, mod); err != nil {
		return nil, err
	}
	if mod.Path == "" {
		return nil, fmt.Errorf("cannot find go module")
	}
	return mod, nil
}

func extractIdent(n ast.Node) *ast.Ident {
	switch n := n.(type) {
	case *ast.Ident:
		return n

	case *ast.StarExpr:
		return extractIdent(n.X)

	case *ast.IndexExpr:
		return extractIdent(n.X)

	case *ast.IndexListExpr:
		return extractIdent(n.X)

	case *ast.SelectorExpr:
		return n.Sel

	default:
		return nil
	}
}

func getFuncDeclReceiver(fset *token.FileSet, decl *ast.FuncDecl) *ast.Ident {
	if decl.Recv == nil {
		return nil
	}

	for _, field := range decl.Recv.List {
		if id := extractIdent(field.Type); id != nil {
			return id
		}
	}

	var buf strings.Builder
	if err := ast.Fprint(&buf, fset, decl.Recv, nil); err != nil {
		panic(err)
	}

	panic(fmt.Sprintf("unable to extract type ident from receiver: %s", buf.String()))
}
