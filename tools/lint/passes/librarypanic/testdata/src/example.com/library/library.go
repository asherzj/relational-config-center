package library

func Bad() {
	panic("boom") // want `library code must not call panic`
}

func Rethrow() {
	defer func() {
		if recovered := recover(); recovered != nil {
			panic(recovered)
		}
	}()
}

func Direct() {
	defer func() {
		panic(recover())
	}()
}

func Assigned() {
	var recovered any = recover()
	panic(recovered)
}

func NotRecovered() {
	recovered := "plain"
	panic(recovered) // want `library code must not call panic`
}

// Reassigned checks librarypanic: the recover-rethrow exemption must not
// survive reassignment. panic of the value still produced by recover is
// allowed; panic after that variable is overwritten is a new panic.
func Reassigned() {
	r := recover()
	panic(r)
	r = "replacement"
	panic(r) // want `library code must not call panic`
}
