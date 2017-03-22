// used reports the most-used identifiers in your code.
package main // import "github.com/sqs/used/cmd/used"

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/loader"

	"github.com/kisielk/gotool"
	"github.com/sqs/used"

	"honnef.co/go/tools/lint"
	"honnef.co/go/tools/lint/lintutil"
)

var (
	fConstants    bool
	fFields       bool
	fFunctions    bool
	fTypes        bool
	fVariables    bool
	fDebug        string
	fWholeProgram bool
	fReflection   bool
	fLintOutput   bool
	fTop          uint
)

func newChecker(mode used.CheckMode) *used.Checker {
	checker := used.NewChecker(mode)

	if fDebug != "" {
		debug, err := os.Create(fDebug)
		if err != nil {
			log.Fatal("couldn't open debug file:", err)
		}
		checker.Debug = debug
	}

	checker.WholeProgram = fWholeProgram
	checker.ConsiderReflection = fReflection
	return checker
}

func main() {
	log.SetFlags(0)

	fs := lintutil.FlagSet("used")
	fs.BoolVar(&fConstants, "consts", true, "Report used constants")
	fs.BoolVar(&fFields, "fields", true, "Report used fields")
	fs.BoolVar(&fFunctions, "funcs", true, "Report used functions and methods")
	fs.BoolVar(&fTypes, "types", true, "Report used types")
	fs.BoolVar(&fVariables, "vars", true, "Report used variables")
	fs.StringVar(&fDebug, "debug", "", "Write a debug graph to `file`. Existing files will be overwritten.")
	fs.BoolVar(&fWholeProgram, "exported", false, "Treat arguments as a program and report used exported identifiers")
	fs.BoolVar(&fReflection, "reflect", true, "Consider identifiers as used when it's likely they'll be accessed via reflection")
	fs.BoolVar(&fLintOutput, "lint-output", false, "Print usage counts for all identifiers in lint format")
	fs.UintVar(&fTop, "top", 5, "Sort lint issues by the usage frequency of their containing definition and show only the first n issues")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	var mode used.CheckMode
	if fConstants {
		mode |= used.CheckConstants
	}
	if fFields {
		mode |= used.CheckFields
	}
	if fFunctions {
		mode |= used.CheckFunctions
	}
	if fTypes {
		mode |= used.CheckTypes
	}
	if fVariables {
		mode |= used.CheckVariables
	}

	checker := newChecker(mode)
	processFlagSet(checker, fs)
}

func processFlagSet(c *used.Checker, fs *flag.FlagSet) {
	tags := fs.Lookup("tags").Value.(flag.Getter).Get().(string)
	ignore := fs.Lookup("ignore").Value.(flag.Getter).Get().(string)
	tests := fs.Lookup("tests").Value.(flag.Getter).Get().(bool)
	lintOutput := fs.Lookup("lint-output").Value.(flag.Getter).Get().(bool)
	top := fs.Lookup("top").Value.(flag.Getter).Get().(uint)

	ignores, err := parseIgnore(ignore)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runner := &runner{
		checker: c,
		tags:    strings.Fields(tags),
		ignores: ignores,
	}
	paths := gotool.ImportPaths(fs.Args())
	goFiles, err := runner.resolveRelative(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		runner.unclean = true
	}
	ctx := build.Default
	ctx.BuildTags = runner.tags
	conf := &loader.Config{
		Build:      &ctx,
		ParserMode: parser.ParseComments,
		ImportPkgs: map[string]bool{},
	}

	var lprog *loader.Program
	if goFiles {
		conf.CreateFromFilenames("adhoc", paths...)
		var err error
		lprog, err = conf.Load()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		for _, path := range paths {
			conf.ImportPkgs[path] = tests
		}
		var err error
		lprog, err = conf.Load()
		if err != nil {
			log.Fatal(err)
		}
	}

	us := runner.check(lprog)
	sort.SliceStable(us, func(i, j int) bool { return us[i].N > us[j].N }) // largest N first
	if lintOutput {
		ps := runner.lint(lprog, us)
		for _, p := range ps {
			runner.unclean = true
			pos := lprog.Fset.Position(p.Pos.Pos())
			fmt.Printf("%v: %s\n", relativePositionString(pos), p.Text)
		}
	} else {
		var more func() bool
		if top > 0 {
			more = func() bool {
				top--
				return top > 0
			}
		} else {
			more = func() bool { return true }
		}
		filterLintInput(lprog.Fset, us, more)
	}
	if runner.unclean {
		os.Exit(1)
	}
}

type gometalinterIssue struct {
	Linter   string `json:"linter"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Message  string `json:"message"`
}

func (i gometalinterIssue) position() token.Position {
	return token.Position{Filename: i.Path, Line: i.Line, Column: i.Col}
}

func filterLintInput(fset *token.FileSet, us []used.Used, more func() bool) {
	objScope := func(obj types.Object) *types.Scope {
		o, ok := obj.(interface {
			Scope() *types.Scope
		})
		if ok {
			return o.Scope()
		}
		return nil
	}

	containsPosition := func(rng interface {
		Pos() token.Pos
		End() token.Pos
	}, p token.Position) bool {
		start := fset.Position(rng.Pos())
		end := fset.Position(rng.End())
		if start.Filename != end.Filename {
			panic(fmt.Sprintf("start.Filename (%s) != end.Filename (%s)", start.Filename, end.Filename))
		}
		return filepath.Base(p.Filename) == filepath.Base(start.Filename) && // TODO(sqs): filepath.Base is not correct
			p.Line >= start.Line && p.Line <= end.Line
		// ((p.Line == start.Line && p.Column >= start.Column) || p.Line > start.Line) &&
		// ((p.Line == end.Line && p.Column <= end.Column) || p.Line < end.Line)
	}

	isInScope := func(us []used.Used, issue gometalinterIssue) bool {
		p := issue.position()
		for _, u := range us {
			scope := objScope(u.Obj)
			if scope == nil {
				continue // TODO(sqs): omits types
			}
			if scope.Pos() == token.NoPos || scope.End() == token.NoPos {
				continue // TODO(sqs)
			}
			//log.Printf("%v has range %v - %v", u.Obj, fset.Position(scope.Pos()), fset.Position(scope.End()))
			if containsPosition(scope, p) {
				//log.Printf("%v in scope because for %v", p, u.Obj)
				return true
			}
			//log.Printf("%v NOT IN RANGE %v - %v", issue, fset.Position(scope.Pos()), fset.Position(scope.End()))
		}
		return false
	}

	// TODO(sqs): detect if linter output is in text or JSON format and handle both
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 || bytes.Equal(line, []byte("[")) || bytes.Equal(line, []byte("]")) {
			continue
		}
		var issue gometalinterIssue
		line = bytes.TrimSuffix(line, []byte(","))
		if err := json.Unmarshal(line, &issue); err != nil {
			log.Fatalf("%s (line was: %q)", err, line)
		}
		if isInScope(us, issue) {
			_, _ = os.Stdout.Write(line)
			_, _ = os.Stdout.Write([]byte("\n"))
			if !more() {
				break
			}
		}
	}
	if s.Err() != nil {
		log.Fatal(s.Err())
	}
}

func parseIgnore(s string) ([]lint.Ignore, error) {
	var out []lint.Ignore
	if len(s) == 0 {
		return nil, nil
	}
	for _, part := range strings.Fields(s) {
		p := strings.Split(part, ":")
		if len(p) != 2 {
			return nil, errors.New("malformed ignore string")
		}
		path := p[0]
		checks := strings.Split(p[1], ",")
		out = append(out, lint.Ignore{Pattern: path, Checks: checks})
	}
	return out, nil
}

func shortPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	if rel, err := filepath.Rel(cwd, path); err == nil && len(rel) < len(path) {
		return rel
	}
	return path
}

func relativePositionString(pos token.Position) string {
	s := shortPath(pos.Filename)
	if pos.IsValid() {
		if s != "" {
			s += ":"
		}
		s += fmt.Sprintf("%d:%d", pos.Line, pos.Column)
	}
	if s == "" {
		s = "-"
	}
	return s
}

type runner struct {
	checker *used.Checker
	tags    []string
	ignores []lint.Ignore

	unclean bool
}

func (runner runner) resolveRelative(importPaths []string) (goFiles bool, err error) {
	if len(importPaths) == 0 {
		return false, nil
	}
	if strings.HasSuffix(importPaths[0], ".go") {
		// User is specifying a package in terms of .go files, don't resolve
		return true, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return false, err
	}
	ctx := build.Default
	ctx.BuildTags = runner.tags
	for i, path := range importPaths {
		bpkg, err := ctx.Import(path, wd, build.FindOnly)
		if err != nil {
			return false, fmt.Errorf("can't load package %q: %v", path, err)
		}
		importPaths[i] = bpkg.ImportPath
	}
	return false, nil
}

func (runner *runner) check(lprog *loader.Program) []used.Used {
	return runner.checker.Check(lprog)
}

func (runner *runner) lint(lprog *loader.Program, us []used.Used) []used.Problem {
	l := used.NewLintData(us)
	return l.LintProblems(lprog)
}
