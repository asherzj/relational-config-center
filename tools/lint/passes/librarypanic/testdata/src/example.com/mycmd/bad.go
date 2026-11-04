package mycmd

func Bad() {
	panic("directory name is not the cmd segment") // want `library code must not call panic`
}
