package pkg

type t struct{} // MATCH /t is used 0 times/

func (t) fragment() {}

func fn() bool { // MATCH /fn is used 0 times/
	var v interface{} = t{}
	switch obj := v.(type) {
	// XXX it shouldn't report fragment(), because fn is used 0 times
	case interface {
		fragment() // MATCH /fragment is used 0 times/
	}:
		obj.fragment()
	}
	return false
}
