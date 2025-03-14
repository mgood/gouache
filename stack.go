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
	visits      *Visit
	turnCount   int
	choiceCount int
	globals     *Vars
	evalStack   *EvalFrame
	listDefs    ListDefs

	locals     *Vars
	callDepth  int
	isFunction bool
	prev       *CallFrame
	returnTo   Element
	retStep    Stepper
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

func (f *CallFrame) ChoiceCount() int {
	if f == nil {
		return 0
	}
	return f.choiceCount
}

func (f *CallFrame) IncChoiceCount() *CallFrame {
	if f == nil {
		return &CallFrame{choiceCount: 1}
	}
	r := *f
	r.choiceCount++
	return &r
}

func (f *CallFrame) ResetChoiceCount() *CallFrame {
	if f == nil {
		return nil
	}
	r := *f
	r.choiceCount = 0
	return &r
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
	// if this is a nested reference, we need to find the deepest location to
	// update
	for {
		nestedRef, ok := f.getRef(ref).(VarRef)
		if !ok {
			break
		}
		ref = nestedRef
	}
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

func (f *CallFrame) PushFrame(returnTo Element, retStep Stepper, isFunction bool) *CallFrame {
	if f == nil {
		return &CallFrame{}
	}
	r := &CallFrame{
		prev:        f,
		returnTo:    returnTo,
		retStep:     retStep,
		visits:      f.visits,
		turnCount:   f.turnCount,
		choiceCount: f.choiceCount,
		globals:     f.globals,
		callDepth:   f.callDepth + 1,
		isFunction:  isFunction,
		evalStack:   f.evalStack,
		listDefs:    f.listDefs,
	}
	return r
}

func (f *CallFrame) PopFrame() (*CallFrame, Element, Stepper, bool) {
	p := f.prev
	if p == nil {
		p = &CallFrame{}
	}
	return &CallFrame{
		visits:      f.visits,
		turnCount:   f.turnCount,
		choiceCount: f.choiceCount,
		globals:     f.globals,
		evalStack:   f.evalStack,
		listDefs:    f.listDefs,

		callDepth:  p.callDepth,
		locals:     p.locals,
		prev:       p.prev,
		returnTo:   p.returnTo,
		retStep:    p.retStep,
		isFunction: p.isFunction,
	}, f.returnTo, f.retStep, f.isFunction
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
		if prev, ok := f.locals.Get(name); ok {
			// this is mainly to allow list values to continue tracking the origin
			// after updating to an empty list
			if u, ok := prev.(interface {
				Updated(Value) Value
			}); ok {
				v = u.Updated(v)
			}
			return f.WithLocal(name, v)
		}
	}
	if prev, ok := f.globals.Get(name); ok {
		// this is mainly to allow list values to continue tracking the origin
		// after updating to an empty list
		if u, ok := prev.(interface {
			Updated(Value) Value
		}); ok {
			v = u.Updated(v)
		}
	}
	return f.WithGlobal(name, v)
}

func (f *CallFrame) resolveRef(v Value) Value {
	for {
		r, ok := v.(VarRef)
		if !ok {
			return v
		}
		v = f.getRef(r)
	}
}

func (f *CallFrame) GetVar(name string) (Value, bool) {
	if f == nil {
		return nil, false
	}
	if f.locals != nil {
		if v, ok := f.locals.Get(name); ok {
			return f.resolveRef(v), true
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
