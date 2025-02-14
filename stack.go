package gouache

import (
	"fmt"
	"strings"
)

type Visit struct {
	Address    Address
	EntryIndex int
	EntryTurn  int
	Prev       *Visit
}

func (v *Visit) Push(addr Address, index, turn int) *Visit {
	if v != nil && v.Address == addr {
		return v
	}
	return &Visit{
		Address:    addr,
		EntryIndex: index,
		EntryTurn:  turn,
		Prev:       v,
	}
}

func (v *Visit) Count(addr Address) int {
	var count int
	entered := false
	for ; v != nil; v = v.Prev {
		if !addr.Contains(v.Address) {
			entered = false
		} else if !entered {
			count++
			entered = true
		}
	}
	return count
}

func (v *Visit) LastVisited(addr Address, turn int) int {
	for ; v != nil; v = v.Prev {
		// this looks at the parent for cases like "start.0" where we entered
		// "start" in a child container, but didn't track the container entry though
		// this should probably have a more precise definition for how we track the
		// container entry
		if addr == v.Address || addr == v.Address.Parent() {
			return v.EntryTurn
		}
	}
	return -1
}

type VarsFrame struct {
	vars *Vars
	prev *VarsFrame
}

func (f *VarsFrame) With(name string, value Value) *VarsFrame {
	if f == nil {
		f = &VarsFrame{}
	}
	return &VarsFrame{
		vars: f.vars.With(name, value),
		prev: f.prev,
	}
}

func (f *VarsFrame) Get(name string) (Value, bool) {
	if f == nil {
		return nil, false
	}
	return f.vars.Get(name)
}

func (f *VarsFrame) Push() *VarsFrame {
	return &VarsFrame{prev: f}
}

func (f *VarsFrame) Pop() *VarsFrame {
	return f.prev
}

type Vars struct {
	name  string
	value Value
	prev  *Vars
}

func (v *Vars) With(name string, value Value) *Vars {
	return &Vars{
		name:  name,
		value: value,
		prev:  v,
	}
}

func (v *Vars) Get(name string) (Value, bool) {
	for ; v != nil; v = v.prev {
		if v.name == name {
			return v.value, true
		}
	}
	return nil, false
}

type EvalFrame struct {
	value Value
	prev  *EvalFrame
}

func (f *EvalFrame) Push(v Value) *EvalFrame {
	return &EvalFrame{
		value: v,
		prev:  f,
	}
}

func (f *EvalFrame) Pop() (Value, *EvalFrame) {
	return f.value, f.prev
}

type newlineState int

const (
	newlineNormal newlineState = iota
	newlineSkipFirst
	newlinePending
	newlineBuffered
)

type ListItem struct {
	Name   string
	Origin string
	Value  int
}

func (li ListItem) Add(v Value) Value {
	return ListItem{
		Origin: li.Origin,
		Value:  li.Value + int(v.(IntValue)),
	}
}

func (li ListItem) Output() Output {
	return Output(li.Name)
}

type ListDefs map[string]map[string]int

func (l ListDefs) All(origin string) ListValue {
	o, ok := l[origin]
	r := ListEmpty(origin)
	if !ok {
		return r
	}
	for name, v := range o {
		r = r.Add(ListSingle(origin, name, v))
	}
	return r
}

func (l ListDefs) Value(origin string, value int) ListValue {
	o, ok := l[origin]
	if ok {
		for name, v := range o {
			if v == value {
				return ListSingle(origin, name, value)
			}
		}
	}
	return ListEmpty(origin)
}

func (l ListDefs) Get(name string) (ListValue, bool) {
	if origin, key, ok := strings.Cut(name, "."); ok {
		o, ok := l[origin]
		if !ok {
			return ListEmpty(origin), false
		}
		v, ok := o[key]
		if !ok {
			return ListEmpty(origin), false
		}
		return ListSingle(origin, key, v), true
	}
	for origin, values := range l {
		if value, ok := values[name]; ok {
			return ListSingle(origin, name, value), true
		}
	}
	return ListValue{}, false
}

type CallFrame struct {
	visits    *Visit
	turnCount int
	globals   *Vars
	evalStack *EvalFrame
	listDefs  ListDefs

	locals       *Vars
	callDepth    int
	evalDepth    int
	stringMode   bool
	newlineState newlineState
	prev         *CallFrame
	returnTo     Element
}

func (f *CallFrame) ShouldEmitNewline() (*CallFrame, bool) {
	if f == nil {
		return nil, true
	}
	switch f.newlineState {
	case newlineNormal:
		return f, true
	case newlineSkipFirst:
		r := *f
		r.newlineState = newlinePending
		return &r, false
	case newlinePending:
		r := *f
		r.newlineState = newlineBuffered
		return &r, false
	case newlineBuffered:
		return f, false
	default:
		panic(fmt.Errorf("unhandled newline state %d", f.newlineState))
	}
}

func (f *CallFrame) ShouldPrependNewline() (*CallFrame, bool) {
	if f == nil {
		return nil, false
	}
	switch f.newlineState {
	case newlineNormal, newlinePending:
		return f, false
	case newlineSkipFirst:
		r := *f
		r.newlineState = newlinePending
		return &r, false
	case newlineBuffered:
		r := *f
		r.newlineState = newlinePending
		return &r, true
	default:
		panic(fmt.Errorf("unhandled newline state %d", f.newlineState))
	}
}

func (f *CallFrame) Visit(addr Address, index int) *CallFrame {
	if f == nil {
		f = &CallFrame{}
	}
	visits := f.visits.Push(addr, index, f.turnCount)
	if f.visits == visits {
		return f
	}
	r := *f
	r.visits = visits
	return &r
}

func (f *CallFrame) TurnsSince(addr Address) int {
	if f == nil {
		return -1
	}
	lastVisit := f.visits.LastVisited(addr, f.turnCount)
	if lastVisit == -1 {
		return -1
	}
	return f.turnCount - lastVisit
}

func (f *CallFrame) VisitCount(addr Address) int {
	if f == nil {
		return 0
	}
	return f.visits.Count(addr)
}

func (f *CallFrame) ListInt(origin string, value int) ListValue {
	return f.listDefs.Value(origin, value)
}

func (f *CallFrame) ListAll(v ListValue) ListValue {
	if len(v.Origins) == 0 {
		return v
	}
	var r ListValue
	for origin := range v.Origins {
		r = r.Add(f.listDefs.All(origin))
	}
	return r
}

func (f *CallFrame) PushVal(v Value) *CallFrame {
	if li, ok := v.(ListValue); ok {
		v = li.Resolve(f.listDefs)
	}
	return f.updateEvalStack(func(s *EvalFrame) *EvalFrame { return s.Push(v) })
}

func (f *CallFrame) PopVal() (Value, *CallFrame) {
	var v Value
	f = f.updateEvalStack(func(s *EvalFrame) *EvalFrame {
		v, s = s.Pop()
		return s
	})
	return v, f
}

func (f *CallFrame) IncTurnCount() *CallFrame {
	if f == nil {
		return &CallFrame{turnCount: 1}
	}
	r := *f
	r.turnCount++
	return &r
}

func (f *CallFrame) setRef(ref VarRef, v Value) *CallFrame {
	if ref.ContentIndex == 0 {
		return f.withGlobals(f.globals.With(ref.Name, v))
	}
	if ref.ContentIndex == f.callDepth+1 {
		return f.withLocals(f.locals.With(ref.Name, v))
	}
	r := *f
	r.prev = f.prev.setRef(ref, v)
	return &r
}

func (f *CallFrame) WithLocal(name string, value Value) *CallFrame {
	v, ok := f.locals.Get(name)
	if ok {
		if r, ok := v.(VarRef); ok {
			return f.setRef(r, value)
		}
	}
	return f.withLocals(f.locals.With(name, value))
}

func (f *CallFrame) PushVarRef(name string) *CallFrame {
	if f == nil {
		panic("uninitialized frame does not have any vars")
	}
	if _, isLocal := f.locals.Get(name); isLocal {
		return f.PushVal(VarRef{Name: name, ContentIndex: f.callDepth + 1})
	}
	if _, isGlobal := f.globals.Get(name); isGlobal {
		return f.PushVal(VarRef{Name: name, ContentIndex: 0})
	}
	panic(fmt.Errorf("variable %s not found", name))
}

func (f *CallFrame) PushFrame(returnTo Element, isFunction bool) *CallFrame {
	if f == nil {
		return &CallFrame{}
	}
	r := &CallFrame{
		prev:      f,
		returnTo:  returnTo,
		visits:    f.visits,
		turnCount: f.turnCount,
		globals:   f.globals,
		callDepth: f.callDepth + 1,
		evalStack: f.evalStack,
		listDefs:  f.listDefs,
	}
	if isFunction {
		r.newlineState = newlineSkipFirst
	}
	return r
}

func (f *CallFrame) PopFrame() (*CallFrame, Element) {
	p := f.prev
	if p == nil {
		p = &CallFrame{}
	}
	return &CallFrame{
		visits:    f.visits,
		turnCount: f.turnCount,
		globals:   f.globals,
		evalStack: f.evalStack,
		listDefs:  f.listDefs,

		callDepth:  p.callDepth,
		locals:     p.locals,
		evalDepth:  p.evalDepth,
		stringMode: p.stringMode,
		prev:       p.prev,
		returnTo:   p.returnTo,
	}, f.returnTo
}

func (f *CallFrame) WithGlobal(name string, value Value) *CallFrame {
	// TODO globals should be declared at start?
	// check for setting an undeclared global?
	return f.withGlobals(f.globals.With(name, value))
}

func (f *CallFrame) getRef(r VarRef) Value {
	if r.ContentIndex == 0 {
		v, ok := f.globals.Get(r.Name)
		if !ok {
			panic(fmt.Errorf("global %s not found", r.Name))
		}
		return v
	}
	if r.ContentIndex == f.callDepth+1 {
		v, ok := f.locals.Get(r.Name)
		if !ok {
			panic(fmt.Errorf("local %s not found", r.Name))
		}
		return v
	}
	return f.prev.getRef(r)
}

func (f *CallFrame) UpdateVar(name string, v Value) *CallFrame {
	if f == nil {
		panic("uninitialized frame does not have any vars")
	}
	if f.locals != nil {
		if _, ok := f.locals.Get(name); ok {
			return f.WithLocal(name, v)
		}
	}
	return f.WithGlobal(name, v)
}

func (f *CallFrame) GetVar(name string) (Value, bool) {
	if f == nil {
		return nil, false
	}
	if f.locals != nil {
		if v, ok := f.locals.Get(name); ok {
			r, ok := v.(VarRef)
			if !ok {
				return v, true
			}
			return f.getRef(r), true
		}
	}
	if f.globals != nil {
		if v, ok := f.globals.Get(name); ok {
			return v, true
		}
	}
	return f.listDefs.Get(name)
}

func (f *CallFrame) withGlobals(v *Vars) *CallFrame {
	if f == nil {
		return &CallFrame{globals: v}
	}
	r := *f
	r.globals = v
	return &r
}

func (f *CallFrame) withLocals(v *Vars) *CallFrame {
	if f == nil {
		return &CallFrame{locals: v}
	}
	r := *f
	r.locals = v
	return &r
}

func (f *CallFrame) updateEvalStack(fn func(*EvalFrame) *EvalFrame) *CallFrame {
	if f == nil {
		return &CallFrame{evalStack: fn(nil)}
	}
	r := *f
	r.evalStack = fn(f.evalStack)
	return &r
}

func (f *CallFrame) IncEvalDepth(by int) *CallFrame {
	if f == nil {
		return &CallFrame{evalDepth: by}
	}
	r := *f
	r.evalDepth += by
	return &r
}

func (f *CallFrame) WithStringMode(on bool) *CallFrame {
	if f == nil {
		return &CallFrame{stringMode: on}
	}
	r := *f
	r.stringMode = on
	return &r
}
