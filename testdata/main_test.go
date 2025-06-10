package main_test

import (
	"example/types"
	"testing"
)

func TestMain(t *testing.T) {
	_ = types.UsedConst
	_ = types.UsedVar
	_ = types.UsedFunc
	types.UnusedVar = 2
}
