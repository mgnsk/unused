package main

import (
	_ "example/testdata/testpkg"
	"example/types"
)

func _() {
	_ = types.UsedInt(0)
	_ = types.UsedStruct{}
	_ = types.UsedGeneric[string]{}
	_ = types.UsedGenericMulti[string, string]{}
}
