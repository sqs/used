package main

type state func() state // MATCH /state is used 1 time/

func a() state { // MATCH /a is used 3 times/
	return a
}

func main() { // MATCH /main is used 2 times/
	st := a // MATCH /st is used 1 time/
	_ = st()
}

type t1 struct{} // MATCH /t1 is used 0 times/
type t2 struct{} // MATCH /t2 is used 1 time/
type t3 struct{} // MATCH /t3 is used 1 time/

func fn1() t1     { return t1{} } // MATCH /fn1 is used 0 times/
func fn2() (x t2) { return }      // MATCH /fn2 is used 2 times/
// MATCH:19 /x is used 1 time/
func fn3() *t3 { return nil } // MATCH /fn3 is used 2 times/

func fn4() { // MATCH /fn4 is used 2 times/
	const x = 1  // MATCH /x is used 1 time/
	const y = 2  // MATCH /y is used 0 times/
	type foo int // MATCH /foo is used 0 times/
	type bar int // MATCH /bar is used 2 times/

	_ = x
	var _ bar
}

func init() { // MATCH /init is used 2 times/
	fn2()
	fn3()
	fn4()
}
