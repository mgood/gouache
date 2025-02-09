package gouache

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

type CallFrame struct {
	visits     *Visit
	turnCount  int
	globals    *Vars
	locals     *Vars
	evalStack  *EvalFrame
	evalDepth  int
	stringMode bool
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

func (f *CallFrame) VisitCount(addr Address) int {
	if f == nil {
		return 0
	}
	return f.visits.Count(addr)
}

func (f *CallFrame) PushVal(v Value) *CallFrame {
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

func (f *CallFrame) WithLocal(name string, value Value) *CallFrame {
	return f.withLocals(f.locals.With(name, value))
}

func (f *CallFrame) WithGlobal(name string, value Value) *CallFrame {
	// TODO globals should be declared at start?
	// check for setting an undeclared global?
	return f.withGlobals(f.globals.With(name, value))
}

func (f *CallFrame) GetVar(name string) (Value, bool) {
	if f == nil {
		return nil, false
	}
	if f.locals != nil {
		if v, ok := f.locals.Get(name); ok {
			return v, true
		}
	}
	if f.globals != nil {
		return f.globals.Get(name)
	}
	return nil, false
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
