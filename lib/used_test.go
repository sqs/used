package used

// Copyright (c) 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found at
// https://developers.google.com/open-source/licenses/bsd.

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/loader"
	"honnef.co/go/tools/lint"
)

var lintMatch = flag.String("match", "", "restrict testdata matches to this pattern")

func TestAll(t *testing.T) {
	baseDir := "testdata"
	fis, err := ioutil.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("ioutil.ReadDir: %v", err)
	}
	if len(fis) == 0 {
		t.Fatalf("no files in %v", baseDir)
	}
	rx, err := regexp.Compile(*lintMatch)
	if err != nil {
		t.Fatalf("Bad -match value %q: %v", *lintMatch, err)
	}
	fis = filterMatches(rx, fis)

	conf := &loader.Config{
		ParserMode: parser.ParseComments,
	}
	sources := map[string][]byte{}
	for _, fi := range fis {
		filename := path.Join(baseDir, fi.Name())
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			t.Errorf("Failed reading %s: %v", fi.Name(), err)
			continue
		}
		f, err := conf.ParseFile(filename, src)
		if err != nil {
			t.Errorf("error parsing %s: %s", filename, err)
			continue
		}
		sources[fi.Name()] = src
		conf.CreateFromFiles(fi.Name(), f)
	}

	lprog, err := conf.Load()
	if err != nil {
		t.Fatalf("error loading program: %s", err)
	}

	checker := NewChecker(CheckAll)
	l := &lint.Linter{Checker: NewLintChecker(checker)}
	res := l.Lint(lprog)
	for _, fi := range fis {
		name := fi.Name()
		src := sources[name]

		ins := parseInstructions(t, name, src)

		for _, in := range ins {
			ok := false
			for i, p := range res {
				pos := lprog.Fset.Position(p.Position)
				if pos.Line != in.Line || filepath.Base(pos.Filename) != name {
					continue
				}
				if in.Match.MatchString(p.Text) {
					// remove this problem from ps
					copy(res[i:], res[i+1:])
					res = res[:len(res)-1]

					//t.Logf("/%v/ matched at %s:%d", in.Match, fi.Name(), in.Line)
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("Lint failed at %s:%d; /%v/ did not match", name, in.Line, in.Match)
			}
		}
	}
	for _, p := range res {
		pos := lprog.Fset.Position(p.Position)
		name := filepath.Base(pos.Filename)
		for _, fi := range fis {
			if name == fi.Name() {
				t.Errorf("Unexpected problem at %s: %v", pos, p.Text)
				break
			}
		}
	}
}

func filterMatches(rx *regexp.Regexp, fis []os.FileInfo) []os.FileInfo {
	matches := fis[:0]
	for _, fi := range fis {
		if rx.MatchString(fi.Name()) {
			matches = append(matches, fi)
		}
	}
	return matches
}

type instruction struct {
	Line        int            // the line number this applies to
	Match       *regexp.Regexp // what pattern to match
	Replacement string         // what the suggested replacement line should be
}

// parseInstructions parses instructions from the comments in a Go source file.
// It returns nil if none were parsed.
func parseInstructions(t *testing.T, filename string, src []byte) []instruction {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Test file %v does not parse: %v", filename, err)
	}
	var ins []instruction
	for _, cg := range f.Comments {
		ln := fset.Position(cg.Pos()).Line
		raw := cg.Text()
		for _, line := range strings.Split(raw, "\n") {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if line == "OK" && ins == nil {
				// so our return value will be non-nil
				ins = make([]instruction, 0)
				continue
			}
			if !strings.Contains(line, "MATCH") {
				continue
			}
			rx, err := extractPattern(line)
			if err != nil {
				t.Fatalf("At %v:%d: %v", filename, ln, err)
			}
			matchLine := ln
			if i := strings.Index(line, "MATCH:"); i >= 0 {
				// This is a match for a different line.
				lns := strings.TrimPrefix(line[i:], "MATCH:")
				lns = lns[:strings.Index(lns, " ")]
				matchLine, err = strconv.Atoi(lns)
				if err != nil {
					t.Fatalf("Bad match line number %q at %v:%d: %v", lns, filename, ln, err)
				}
			}
			var repl string
			if r, ok := extractReplacement(line); ok {
				repl = r
			}
			ins = append(ins, instruction{
				Line:        matchLine,
				Match:       rx,
				Replacement: repl,
			})
		}
	}
	return ins
}

func extractPattern(line string) (*regexp.Regexp, error) {
	n := strings.Index(line, " ")
	if n == 01 {
		return nil, fmt.Errorf("malformed match instruction %q", line)
	}
	line = line[n+1:]
	var pat string
	switch line[0] {
	case '/':
		a, b := strings.Index(line, "/"), strings.LastIndex(line, "/")
		if a == -1 || a == b {
			return nil, fmt.Errorf("malformed match instruction %q", line)
		}
		pat = line[a+1 : b]
	case '"':
		a, b := strings.Index(line, `"`), strings.LastIndex(line, `"`)
		if a == -1 || a == b {
			return nil, fmt.Errorf("malformed match instruction %q", line)
		}
		pat = regexp.QuoteMeta(line[a+1 : b])
	default:
		return nil, fmt.Errorf("malformed match instruction %q", line)
	}

	rx, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("bad match pattern %q: %v", pat, err)
	}
	return rx, nil
}

func extractReplacement(line string) (string, bool) {
	// Look for this:  / -> `
	// (the end of a match and start of a backtick string),
	// and then the closing backtick.
	const start = "/ -> `"
	a, b := strings.Index(line, start), strings.LastIndex(line, "`")
	if a < 0 || a > b {
		return "", false
	}
	return line[a+len(start) : b], true
}
