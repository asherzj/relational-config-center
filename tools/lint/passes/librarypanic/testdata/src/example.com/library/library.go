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
