package gouache

import (
	"fmt"
	"strings"
)

const InkVersion = 21

type ContainerFlag uint32

const (
	RecordVisits   ContainerFlag = 0x1 // The story should keep a record of the number of visits to this container.
	CountTurns     ContainerFlag = 0x2 // The story should keep a record of the number of the turn index that this container was lasted visited.
	CountStartOnly ContainerFlag = 0x4 // For the above numbers, the story should only record changes when the story visits the very first subelement, rather than random entry at any point. Used to distinguish the different behaviour between knots and stitches (random access), versus gather points and choices (count start only).
)

type Node interface{}

type Text string
type Newline struct{} // "\n"
type Address string

func (a Address) Parent() Address {
	i := strings.LastIndex(string(a), ".")
	if i == -1 {
		return ""
	}
	return Address(a[:i])
}

func (a Address) Contains(b Address) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(string(b), string(a)+".")
}

// Tries to close/pop the active thread, otherwise marks the story flow safe to exit without a loose end warning.
type Done struct{}

// Ends the story flow immediately, closes all active threads, unwinds the callstack, and removes any choices that were previously created.
type End struct{}

type ChoicePointFlag uint32

const (
	HasCondition         ChoicePointFlag = 0x01 // Has condition?: Set if the story should pop a value from the evaluation stack in order to determine whether a choice instance should be created at all.
	HasStartContent      ChoicePointFlag = 0x02 // Has start content? - According to square bracket notation, is there any leading content before any square brackets? If so, this content should be popped from the evaluation stack.
	HasChoiceOnlyContent ChoicePointFlag = 0x04 // Has choice-only content? - According to square bracket notation, is there any content between the square brackets? If so, this content should be popped from the evaluation stack.
	IsInvisibleDefault   ChoicePointFlag = 0x08 // Is invisible default? - When this is enabled, the choice isn't provided to the game (isn't presented to the player), and instead is automatically followed if there are no other choices generated.
	OnceOnly             ChoicePointFlag = 0x10 // Once only? - Defaults to true. This is the difference between the * and + choice bullets in ink. If once only (*), the choice is only displayed if its target container's read count is zero.
)

type ChoicePoint struct {
	Dest  Address         `json:"*"`
	Flags ChoicePointFlag `json:"flg"`
}
type Divert struct {
	Dest         Address `json:"->"`
	Var          bool    `json:"var"`
	Conditional  bool    `json:"c"`
	incTurnCount bool
}

type FuncCall struct {
	Dest Address `json:"f()"`
}

type BeginEval struct{}       // "ev"
type EndEval struct{}         // "/ev"
type BeginStringEval struct{} // "str"
type EndStringEval struct{}   // "/str"
type BeginTag struct{}        // "#"
type EndTag struct{}          // "/#"
type Out struct{}             // "out"
type Pop struct{}             // "pop"
type NoOp struct{}            // "pop"
type TurnCounter struct{}     // "turn"
type FuncReturn struct{}      // "~ret"
type Void struct{}            // "void"

type UnaryOp func(a Value) Value

var Not UnaryOp = func(a Value) Value {
	switch a := a.(type) {
	case IntValue:
		return boolean(a == 0)
	default:
		panic("unsupported type")
	}
}

var Neg UnaryOp = func(a Value) Value {
	switch a := a.(type) {
	case IntValue:
		return -a
	case FloatValue:
		return -a
	default:
		panic("unsupported type")
	}
}

type BinOp func(a, b Value) Value

var Add BinOp = func(a, b Value) Value {
	switch a := a.(type) {
	case FloatValue:
		return a + b.(FloatValue)
	case IntValue:
		return a + b.(IntValue)
	default:
		panic("unsupported type")
	}
}

var Sub BinOp = func(a, b Value) Value {
	switch a := a.(type) {
	case FloatValue:
		return a - b.(FloatValue)
	case IntValue:
		return a - b.(IntValue)
	default:
		panic("unsupported type")
	}
}

var Div BinOp = func(a, b Value) Value {
	switch a := a.(type) {
	case FloatValue:
		return a / b.(FloatValue)
	case IntValue:
		return a / b.(IntValue)
	default:
		panic("unsupported type")
	}
}

var Mul BinOp = func(a, b Value) Value {
	switch a := a.(type) {
	case FloatValue:
		return a * b.(FloatValue)
	case IntValue:
		return a * b.(IntValue)
	default:
		panic("unsupported type")
	}
}

var Mod BinOp = func(a, b Value) Value {
	switch a := a.(type) {
	case IntValue:
		return a % b.(IntValue)
	default:
		panic("unsupported type")
	}
}

var Eq BinOp = func(a, b Value) Value {
	return boolean(a == b)
}

var Ne BinOp = func(a, b Value) Value {
	return boolean(a != b)
}

var And BinOp = func(a, b Value) Value {
	return boolean(truthy(a) && truthy(b))
}

var Or BinOp = func(a, b Value) Value {
	return boolean(truthy(a) || truthy(b))
}

var Lt BinOp = func(a, b Value) Value {
	switch a := a.(type) {
	case FloatValue:
		return boolean(a < b.(FloatValue))
	case IntValue:
		return boolean(a < b.(IntValue))
	default:
		panic("unsupported type")
	}
}

var Gt BinOp = func(a, b Value) Value {
	return Lt(b, a)
}

var Lte BinOp = func(a, b Value) Value {
	return Or(
		Lt(a, b),
		Eq(a, b),
	)
}

var Gte BinOp = func(a, b Value) Value {
	return Or(
		Gt(a, b),
		Eq(a, b),
	)
}

var Min BinOp = func(a, b Value) Value {
	if truthy(Lt(b, a)) {
		return b
	}
	return a
}

var Max BinOp = func(a, b Value) Value {
	if truthy(Gt(b, a)) {
		return b
	}
	return a
}

type SetTemp struct {
	Name string `json:"temp="`
}

type SetVar struct {
	Name string `json:"VAR="`
}

type GetVar struct {
	Name string `json:"VAR?"`
}

type GetVisitCount struct {
	Container string `json:"CNT?"`
}

type VarRef struct {
	Name         string `json:"^var"`
	ContentIndex int    `json:"ci"`
}

type Value interface{}

type StringValue string // "^text"

func (s StringValue) Output() Output {
	return Output(s)
}

type VoidValue struct{}

func (v VoidValue) Output() Output {
	return Output("")
}

type FloatValue float64

func (f FloatValue) Output() Output {
	s := fmt.Sprint(f)
	return Output(s)
}

type IntValue int64

func (i IntValue) Output() Output {
	s := fmt.Sprint(i)
	return Output(s)
}

type BoolValue bool

func (b BoolValue) Output() Output {
	s := fmt.Sprint(b)
	return Output(s)
}

func boolean(b bool) BoolValue {
	return BoolValue(b)
}

func truthy(v Value) bool {
	switch v := v.(type) {
	case BoolValue:
		return bool(v)
	case IntValue:
		return v != 0
	default:
		panic("unsupported type")
	}
}

type DivertTargetValue struct {
	Dest Address `json:"^->"`
}
