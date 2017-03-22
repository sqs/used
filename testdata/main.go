package main

func Fn1() {}
func Fn2() {} // MATCH /Fn2 is used 0 times/

const X = 1 // MATCH /X is used 0 times/

var Y = 2 // MATCH /Y is used 0 times/

type Z struct{} // MATCH /Z is used 0 times/

func main() {
	Fn1()
}
