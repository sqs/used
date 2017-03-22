# used - compute the most-used identifiers in a Go program

The `used` tool analyzes a Go program to compute the most-used identifiers.

It can also filter the output of [gometalinter](https://github.com/alecthomas/gometalinter) to show problems only on the most-used identifiers. This lets you use data to prioritize what to fix/improve in a Go program.

## Install

```
go get github.com/sqs/used
```

## Usage

### Show the most-used identifiers in a Go program

```bash
$ cat > /tmp/file.go <<EOF
package foo

func f1() { f2() } // note that f1 has no callers

func f2() {}

func init() {
	f2()
}
EOF

$ used -lint-output /tmp/file.go
/tmp.file.go:3:6: func f1 is used 0 times
/tmp.file.go:5:6: func f2 is used 2 times
```

### Filter `gometalinter` output to show problems only on the most-used identifiers

```bash
$ go get gopkg.in/alecthomas/gometalinter.v1
$ gometalinter.v1 --install
$ go get github.com/gorilla/mux
$ cd $(go list -f '{{.Dir}}' github.com/gorilla/mux)
$ gometalinter.v1 --disable=gocyclo --disable=vetshadow --disable=goconst --json ./... | used -top 5 ./...
  {"linter":"golint","severity":"warning","path":"route.go","line":44,"col":1,"message":"exported method Route.SkipClean should have comment or be unexported"}
  {"linter":"errcheck","severity":"warning","path":"mux_test.go","line":1646,"col":11,"message":"error return value not checked (req.Write(\u0026buff))"}
  {"linter":"gosimple","severity":"warning","path":"mux_test.go","line":948,"col":3,"message":"should use 'return \u003cexpr\u003e' instead of 'if \u003cexpr\u003e { return \u003cbool\u003e }; return \u003cbool\u003e' (S1008)"}
  {"linter":"gosimple","severity":"warning","path":"old_test.go","line":596,"col":5,"message":"should omit comparison to bool constant, can be simplified to route.strictSlash (S1002)"}
  {"linter":"gosimple","severity":"warning","path":"old_test.go","line":602,"col":5,"message":"should omit comparison to bool constant, can be simplified to !route.strictSlash (S1002)"}
```

You can use any existing `gometalinter` flags, except that:

* You must use `gometalinter --json`.
* The file/package arguments to `used` must be a superset of what you pass to `gometalinter` (typically they are the same, and `./...` is most common).

## Acknowledgments

The `used` tool is derived from [Dominik Honnef's `unused` tool](https://github.com/dominikh/go-tools/tree/master/cmd/unused).
