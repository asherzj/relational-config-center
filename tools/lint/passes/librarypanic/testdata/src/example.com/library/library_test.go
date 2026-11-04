package library

import "testing"

func TestAllowsPanic(t *testing.T) {
	if false {
		panic("in test")
	}
}
