package pkg

import _ "fmt" // MATCH /_ is used 1 time/

type t1 struct{} // MATCH /t1 is used 0 times/
type t2 struct{} // MATCH /t2 is used 2 times/
type t3 struct{} // MATCH /t3 is used 2 times/

var _ = t2{}

func fn1() { // MATCH /fn1 is used 0 times/
	_ = t1{}
	var _ = t1{}
}

func fn2() { // MATCH /fn2 is used 2 times/
	_ = t3{}
}

func init() { // MATCH /init is used 2 times/
	fn2()
}

func _() {}

type _ struct{}
