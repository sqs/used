package pkg

func init() {
	var p P
	_ = p.n
}

type T0 struct {
	m int // MATCH /m is used 0 times/
	n int
}

type T1 struct {
	T0
}

type P *T1
