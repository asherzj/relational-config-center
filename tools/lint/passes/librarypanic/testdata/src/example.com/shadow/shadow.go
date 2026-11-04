package shadow

func panic(v any) {}

func Call() {
	panic("local function, not the builtin")
}
