package gouache

import (
	"cmp"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
)

const (
	MinInkVersion = 19
	MaxInkVersion = 21
)

type ContainerFlag uint32

const (
	RecordVisits   ContainerFlag = 0x1 // The story should keep a record of the number of visits to this container.
	CountTurns     ContainerFlag = 0x2 // The story should keep a record of the number of the turn index that this container was lasted visited.
	CountStartOnly ContainerFlag = 0x4 // For the above numbers, the story should only record changes when the story visits the very first subelement, rather than random entry at any point. Used to distinguish the different behaviour between knots and stitches (random access), versus gather points and choices (count start only).
)

type Node interface{}

type Text string
type Newline struct{} // "\n"
type Glue struct{}    // "<>"
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
	Dest        Address `json:"->"`
	Var         bool    `json:"var"`
	Conditional bool    `json:"c"`
}

type FuncCall struct {
	Dest Address `json:"f()"`
	Var  bool    `json:"var"`
}

type TunnelCall struct {
	Dest Address `json:"->t->"`
	Var  bool    `json:"var"`
}

type BeginEval struct{}         // "ev"
type EndEval struct{}           // "/ev"
type BeginStringEval struct{}   // "str"
type EndStringEval struct{}     // "/str"
type BeginTag struct{}          // "#"
type EndTag struct{}            // "/#"
type Out struct{}               // "out"
type Pop struct{}               // "pop"
type DupTop struct{}            // "du"
type NoOp struct{}              // "nop"
type TurnCounter struct{}       // "turn"
type TurnsSince struct{}        // "turns"
type VisitIndex struct{}        // "visit"
type ReadCountFunc struct{}     // "readc"
type ChoiceCounter struct{}     // "choiceCnt"
type FuncReturn struct{}        // "~ret"
type TunnelReturn struct{}      // "->->"
type ThreadStart struct{}       // "thread"
type Void struct{}              // "void"
type ListInt struct{}           // "listInt"
type ListValueFunc struct{}     // "LIST_VALUE"
type ListCountFunc struct{}     // "LIST_COUNT"
type ListMinFunc struct{}       // "LIST_MIN"
type ListMaxFunc struct{}       // "LIST_MAX"
type ListAllFunc struct{}       // "LIST_ALL"
type ListInvertFunc struct{}    // "LIST_INVERT"
type ListIntersectFunc struct{} // "L^"
type ListRangeFunc struct{}     // "range"
type Seq struct{}               // "seq"

type UnaryOp func(a Value) Value

var Not UnaryOp = func(a Value) Value {
	return boolean(!truthy(a))
}

var Neg UnaryOp = func(a Value) Value {
	switch a := a.(type) {
	case IntValue:
		return -a
	case FloatValue:
		return -a
	case BoolValue:
		return -boolInt(a)
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

func floatOp(op func(float64) float64) UnaryOp {
	return func(a Value) Value {
		return FloatValue(op(float64(asFloat(a))))
	}
}

var Floor UnaryOp = floatOp(math.Floor)

var Ceiling UnaryOp = floatOp(math.Ceil)

var Int UnaryOp = func(a Value) Value {
	return IntValue(a.(FloatValue))
}

type BinOp func(a, b Value) Value

var Add BinOp = func(a, b Value) Value {
	// if "a" is a string, this takes precedence over numeric conversions
	switch a := a.(type) {
	case StringValue:
		return a + asStringValue(b)
	}
	switch bt := b.(type) {
	case StringValue:
		return asStringValue(a) + bt
	case FloatValue:
		a = asFloat(a)
	case BoolValue:
		b = boolInt(bt)
	}
	switch a := a.(type) {
	case FloatValue:
		return a + asFloat(b)
	case IntValue:
		return a + b.(IntValue)
	case ListValue:
		return a.Add(b)
	case BoolValue:
		return boolInt(a) + b.(IntValue)
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

func boolInt(b BoolValue) IntValue {
	if b {
		return 1
	}
	return 0
}

func asStringValue(v Value) StringValue {
	if v, ok := v.(StringValue); ok {
		return v
	}
	if v, ok := v.(Outputter); ok {
		return StringValue(v.Output().String())
	}
	panic(fmt.Errorf("unsupported type %T", v))
}

var Has BinOp = func(a, b Value) Value {
	switch a := a.(type) {
	case ListValue:
		return boolean(a.Contains(b.(ListValue)))
	case StringValue:
		return boolean(strings.Contains(string(a), string(asStringValue(b))))
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

var Hasnt BinOp = func(a, b Value) Value {
	return Not(Has(a, b))
}

var Sub BinOp = func(a, b Value) Value {
	switch bt := b.(type) {
	case FloatValue:
		a = asFloat(a)
	case BoolValue:
		b = boolInt(bt)
	}
	switch a := a.(type) {
	case FloatValue:
		return a - asFloat(b)
	case IntValue:
		return a - b.(IntValue)
	case ListValue:
		return a.Sub(b)
	case BoolValue:
		return boolInt(a) - b.(IntValue)
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

var Div BinOp = func(a, b Value) Value {
	switch bt := b.(type) {
	case FloatValue:
		a = asFloat(a)
	case BoolValue:
		b = boolInt(bt)
	}
	switch a := a.(type) {
	case FloatValue:
		return a / asFloat(b)
	case IntValue:
		return a / b.(IntValue)
	case BoolValue:
		return boolInt(a) / b.(IntValue)
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

func asFloat(v Value) FloatValue {
	switch v := v.(type) {
	case FloatValue:
		return v
	case IntValue:
		return FloatValue(v)
	case BoolValue:
		return FloatValue(boolInt(v))
	default:
		panic(fmt.Errorf("unsupported type %T", v))
	}
}

var Mul BinOp = func(a, b Value) Value {
	switch bt := b.(type) {
	case FloatValue:
		a = asFloat(a)
	case BoolValue:
		b = boolInt(bt)
	}
	switch a := a.(type) {
	case FloatValue:
		return a * asFloat(b)
	case IntValue:
		return a * b.(IntValue)
	case BoolValue:
		return boolInt(a) * b.(IntValue)
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

var Mod BinOp = func(a, b Value) Value {
	switch bt := b.(type) {
	case FloatValue:
		a = asFloat(a)
	case BoolValue:
		b = boolInt(bt)
	}
	switch a := a.(type) {
	case FloatValue:
		return FloatValue(math.Mod(float64(a), float64(asFloat(b))))
	case IntValue:
		return a % b.(IntValue)
	case BoolValue:
		return boolInt(a) % b.(IntValue)
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

var Eq BinOp = func(a, b Value) Value {
	if eq, ok := a.(interface {
		Eq(b Value) bool
	}); ok {
		return boolean(eq.Eq(b))
	}
	// if one of the value is a string, try comparing as strings
	if _, ok := a.(StringValue); ok {
		b = asStringValue(b)
	} else if _, ok := b.(StringValue); ok {
		a = asStringValue(a)
	}
	return boolean(a == b)
}

var Ne BinOp = func(a, b Value) Value {
	return Not(Eq(a, b))
}

var And BinOp = func(a, b Value) Value {
	return boolean(truthy(a) && truthy(b))
}

var Or BinOp = func(a, b Value) Value {
	return boolean(truthy(a) || truthy(b))
}

var Lt BinOp = func(a, b Value) Value {
	if a, ok := a.(interface {
		Lt(b Value) bool
	}); ok {
		return boolean(a.Lt(b))
	}
	switch a := a.(type) {
	case FloatValue:
		return boolean(a < b.(FloatValue))
	case IntValue:
		return boolean(a < b.(IntValue))
	default:
		panic(fmt.Errorf("unsupported type %T", a))
	}
}

var Gt BinOp = func(a, b Value) Value {
	if a, ok := a.(interface {
		Gt(b Value) bool
	}); ok {
		return boolean(a.Gt(b))
	}
	return Lt(b, a)
}

var Lte BinOp = func(a, b Value) Value {
	if a, ok := a.(interface {
		Lte(b Value) bool
	}); ok {
		return boolean(a.Lte(b))
	}
	return Or(
		Lt(a, b),
		Eq(a, b),
	)
}

var Gte BinOp = func(a, b Value) Value {
	if a, ok := a.(interface {
		Gte(b Value) bool
	}); ok {
		return boolean(a.Gte(b))
	}
	return Lte(b, a)
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

// TODO the source should be stored in the stack so that if there are
// concurrent stories they each have their own random state.
// However, we'll just use a global random source for now.
// Since PGC uses a simple internal state, we could also copy the struct
// to preserve its state after each call to ensure determinism.
var (
	randSource *rand.Rand
	storySeed  uint64
)

func seedRandom(seed uint64) {
	storySeed = seed
	randSource = rand.New(rand.NewPCG(0, seed))
}

var Rnd BinOp = func(a, b Value) Value {
	if randSource == nil {
		seedRandom(rand.Uint64())
	}
	lo := int64(a.(IntValue))
	hi := int64(b.(IntValue))
	r := lo + randSource.Int64N(hi-lo)
	return IntValue(r)
}

var Srnd UnaryOp = func(v Value) Value {
	seedRandom(uint64(v.(IntValue)))
	return VoidValue{}
}

func shuffle(container string, elements, visitIndex int) int {
	h := fnv.New64a()
	_, err := io.WriteString(h, container)
	if err != nil {
		panic(err)
	}
	loop := visitIndex / elements
	seed := h.Sum64() + uint64(loop)
	src := rand.New(rand.NewPCG(seed, storySeed))
	perm := src.Perm(elements)
	return perm[visitIndex%elements]
}

type SetTemp struct {
	Name     string `json:"temp="`
	Reassign bool   `json:"re"`
}

type SetVar struct {
	Name     string `json:"VAR="`
	Reassign bool   `json:"re"`
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
	// lower precision to match ink text output
	s := fmt.Sprint(float32(f))
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

type ListValue struct {
	Items   []ListItem          `json:"list"`
	Origins map[string]struct{} `json:"origins"`
}

func ListEmpty(origin string) ListValue {
	return ListValue{
		Origins: map[string]struct{}{origin: {}},
	}
}

func ListSingle(origin, name string, value int) ListValue {
	return ListValue{
		Items: []ListItem{
			{Origin: origin, Name: name, Value: value},
		},
		Origins: map[string]struct{}{origin: {}},
	}
}

func (l ListValue) Lt(v Value) bool {
	m := v.(ListValue)
	if len(l.Items) == 0 {
		return len(m.Items) > 0
	}
	if len(m.Items) == 0 {
		return false
	}
	return l.Items[len(l.Items)-1].Value < m.Items[0].Value
}

func (l ListValue) Lte(v Value) bool {
	m := v.(ListValue)
	if len(l.Items) == 0 {
		return len(m.Items) > 0
	}
	if len(m.Items) == 0 {
		return false
	}
	if l.Items[0].Value > m.Items[0].Value {
		return false
	}
	return l.Items[len(l.Items)-1].Value <= m.Items[len(m.Items)-1].Value
}

func (l ListValue) At(index int) ListValue {
	return ListValue{
		Items: []ListItem{
			l.Items[index],
		},
		Origins: map[string]struct{}{
			l.Items[index].Origin: {},
		},
	}
}

func (l ListValue) Updated(v Value) Value {
	lv, ok := v.(ListValue)
	// if it's not a list, or has its own values, return it
	if !ok || len(lv.Items) > 0 {
		return v
	}
	// if we're updating to an empty list, return empty with the previous origins
	return ListValue{
		Origins: maps.Clone(l.Origins),
	}
}

func numeric(v Value) int {
	switch v := v.(type) {
	case IntValue:
		return int(v)
	case ListValue:
		if len(v.Items) != 1 {
			panic(fmt.Errorf("should have 1 item to treat as number"))
		}
		return v.Items[0].Value
	default:
		panic(fmt.Errorf("unexpected type %T", v))
	}
}

func (l ListValue) Range(start, stop Value) ListValue {
	a := numeric(start)
	b := numeric(stop)
	return l.filter(l, func(x ListItem) bool {
		return a <= x.Value && x.Value <= b
	})
}

func (l ListValue) Contains(v ListValue) bool {
	if len(v.Items) == 0 {
		return false
	}
	for _, item := range v.Items {
		if !l.contains(item) {
			return false
		}
	}
	return true
}

func (l ListValue) contains(x ListItem) bool {
	return slices.Contains(l.Items, x)
}

func (l ListValue) Eq(v Value) bool {
	l2, ok := v.(ListValue)
	if !ok {
		return false
	}
	return slices.Equal(l.Items, l2.Items)
}

func (l ListValue) Resolve(defs ListDefs) ListValue {
	r := ListValue{
		Origins: make(map[string]struct{}),
	}
	for _, item := range l.Items {
		if item.Name != "" {
			r.Items = append(r.Items, item)
			r.Origins[item.Origin] = struct{}{}
			continue
		}
		if o, ok := defs[item.Origin]; ok {
			for name, value := range o {
				if value == item.Value {
					r.Items = append(r.Items, ListItem{
						Origin: item.Origin,
						Name:   name,
						Value:  value,
					})
					r.Origins[item.Origin] = struct{}{}
				}
			}
		}
	}
	if len(r.Items) == 0 {
		r.Origins = l.Origins
	}
	return r
}

func (l ListValue) Put(origin, name string, value int) ListValue {
	return l.Add(ListSingle(origin, name, value))
}

func (l ListValue) Add(v Value) ListValue {
	switch v := v.(type) {
	case ListValue:
		return l.merge(v)
	case IntValue:
		return l.inc(int(v))
	default:
		panic(fmt.Errorf("unsupported type %T", v))
	}
}

func (l ListValue) Sub(v Value) ListValue {
	switch v := v.(type) {
	case ListValue:
		return l.diff(v)
	case IntValue:
		return l.inc(-int(v))
	default:
		panic(fmt.Errorf("unsupported type %T", v))
	}
}

func (l ListValue) Intersect(v ListValue) ListValue {
	return l.filter(v, func(x ListItem) bool {
		return v.contains(x)
	})
}

func (l ListValue) inc(v int) ListValue {
	var items []ListItem
	for _, item := range l.Items {
		items = append(items, ListItem{
			Origin: item.Origin,
			Value:  item.Value + v,
		})
	}
	return ListValue{
		Items: items,
	}
}

func (l ListValue) filter(m ListValue, p func(ListItem) bool) ListValue {
	r := ListValue{
		Origins: make(map[string]struct{}),
	}
	for _, item := range l.Items {
		if p(item) {
			r.Items = append(r.Items, item)
			r.Origins[item.Origin] = struct{}{}
		}
	}
	if len(r.Items) == 0 {
		r.Origins = l.Origins
	}
	return r
}

func (l ListValue) diff(m ListValue) ListValue {
	return l.filter(m, func(x ListItem) bool {
		return !m.contains(x)
	})
}

func (l ListValue) merge(m ListValue) ListValue {
	var items []ListItem
	items = append(items, l.Items...)
	items = append(items, m.Items...)
	slices.SortFunc(items, func(a, b ListItem) int {
		if c := cmp.Compare(a.Value, b.Value); c != 0 {
			return c
		}
		return cmp.Compare(a.Origin, b.Origin)
	})
	items = slices.Compact(items)
	r := ListValue{
		Items:   items,
		Origins: make(map[string]struct{}),
	}
	maps.Copy(r.Origins, l.Origins)
	maps.Copy(r.Origins, m.Origins)
	return r
}

func (l ListValue) Output() Output {
	var b strings.Builder
	for i, item := range l.Items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(item.Name)
	}
	return Output(b.String())
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
	case ListValue:
		return len(v.Items) > 0
	default:
		panic(fmt.Errorf("unsupported type %T", v))
	}
}

type DivertTargetValue struct {
	Dest Address `json:"^->"`
}
