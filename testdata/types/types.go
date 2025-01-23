package types

//

type UnusedStruct struct{} // want `unused type UnusedStruct`

func (s UnusedStruct) Method() {}

//

type UnusedStructWithConstructor struct{}

func (s *UnusedStructWithConstructor) Method() {}

func NewUnusedStructWithConstructor() *UnusedStructWithConstructor { // want `unused func NewUnusedStructWithConstructor`
	return &UnusedStructWithConstructor{}
}

//

type UnusedInt int // want `unused type UnusedInt`

//

type UnusedGeneric[T any] struct{} // want `unused type UnusedGeneric`

func (s UnusedGeneric[T]) Method() {}

//

type UnusedGenericMulti[A, B any] struct{} // want `unused type UnusedGenericMulti`

func (s UnusedGenericMulti[A, B]) Method() {}

//

type UnusedGenericWithConstructor[T any] struct{}

func (s *UnusedGenericWithConstructor[T]) Method() {}

func NewUnusedGenericWithConstructor[T any]() *UnusedGenericWithConstructor[T] { // want `unused func NewUnusedGenericWithConstructor`
	return &UnusedGenericWithConstructor[T]{}
}

//

type UnusedGenericMultiWithConstructor[A, B any] struct{}

func (s *UnusedGenericMultiWithConstructor[A, B]) Method() {}

func NewUnusedGenericMultiWithConstructor[A, B any]() *UnusedGenericMultiWithConstructor[A, B] { // want `unused func NewUnusedGenericMultiWithConstructor`
	return &UnusedGenericMultiWithConstructor[A, B]{}
}

// Const.

const (
	UnusedConst = 1 // want `unused const UnusedConst`
	UsedConst   = 1
)

// Var.

var (
	UnusedVar = 1 // want `unused var UnusedVar`
	UsedVar   = 1
)

// Used objects.

func UsedFunc() {}

//

type UsedInt int

//

type UsedStruct struct{}

//

type UsedGeneric[T any] struct{}

//

type UsedGenericMulti[A, B any] struct{}

// Ignored.

func main() {}
func init() {}
func Test() {}
