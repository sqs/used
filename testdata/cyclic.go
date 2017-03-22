package pkg

func a() { // MATCH /a is used 0 times/
	b()
}

func b() { // MATCH /b is used 0 times/
	a()
}
