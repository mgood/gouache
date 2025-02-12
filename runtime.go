package gouache

import (
	"fmt"
	"strings"
)

type Output string

func (o Output) String() string {
	return string(o)
}

type Outputter interface {
	Output() Output
}

type Choice struct {
	Label              string
	Dest               Element
	IsInvisibleDefault bool
}

type Element interface {
	Node() Node
	Address() (Address, int)
	Find(Address) Element
	Next() Element
}

type Evaluator interface {
	Step(Element) (Output, *Choice, Element, Evaluator)
}

func Init(root Element, listDefs ListDefs) Evaluator {
	var eval Evaluator = BaseEvaluator{
		Stack: &CallFrame{
			listDefs: listDefs,
		},
	}
	if g := root.Find("global decl"); g != nil {
		var s Output
		var choice *Choice
		elem := g
		for ; ; s, choice, elem, eval = eval.Step(elem) {
			if s.String() != "" {
				panic(fmt.Errorf("unexpected output while initializing globals %q", s))
			}
			if choice != nil {
				panic(fmt.Errorf("unexpected choice while initializing globals %#v", choice))
			}
			if elem == nil {
				break
			}
		}
	}
	return eval
}

// elements should report their path
// track number of elements visited in this parent
// when parent changes, record the visits for the container
// though follow the flags on the parent to determine when to update
// RecordVisits
// CountTurns
// CountStartOnly

type BaseEvaluator struct {
	Stack *CallFrame
}

func popIfEnded(s *CallFrame, next Element) (*CallFrame, Element) {
	if next != nil {
		return s, next
	}
	s, next = s.PopFrame()
	s = s.PushVal(VoidValue{})
	return s, next
}

func (e BaseEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
	e.Stack = e.Stack.Visit(el.Address())
	switch n := el.Node().(type) {
	case Text:
		s, newline := e.Stack.ShouldPrependNewline()
		if newline {
			n = "\n" + n
		}
		s, next := popIfEnded(s, el.Next())
		return Output(n), nil, next, endEval(s)
	case Newline:
		s, emit := e.Stack.ShouldEmitNewline()
		var o Output
		if emit {
			o = Output("\n")
		}
		s, next := popIfEnded(s, el.Next())
		return o, nil, next, endEval(s)
	case BeginEval:
		s := e.Stack.IncEvalDepth(1)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case SetTemp:
		val, s := e.Stack.PopVal()
		s = s.WithLocal(n.Name, val)
		s, next := popIfEnded(s, el.Next())
		return "", nil, next, endEval(s)
	case Pop:
		_, s := e.Stack.PopVal()
		s, next := popIfEnded(s, el.Next())
		return "", nil, next, endEval(s)
	case DupTop:
		v, s := e.Stack.PopVal()
		s = s.PushVal(v)
		s = s.PushVal(v)
		s, next := popIfEnded(s, el.Next())
		return "", nil, next, endEval(s)
	case Divert:
		addr := n.Dest
		if n.Var {
			addrVar, ok := e.Stack.GetVar(string(addr))
			if !ok {
				panic(fmt.Errorf("address variable %q not found", addr))
			}
			addr = addrVar.(DivertTargetValue).Dest
		}
		if n.Conditional {
			var cond Value
			cond, e.Stack = e.Stack.PopVal()
			if !truthy(cond) {
				return "", nil, el.Next(), e
			}
		}
		if n.incTurnCount {
			e.Stack = e.Stack.IncTurnCount()
		}
		dest := el.Find(addr)
		if dest == nil {
			panic(fmt.Errorf("divert target %q not found", n.Dest))
		}
		return "", nil, dest, e
	case BeginTag:
		return "", nil, el.Next(), TagEvaluator{Stack: e.Stack}
	case ChoicePoint:
		var label StringValue
		enabled := true
		s := e.Stack
		if n.Flags&HasCondition != 0 {
			var cond Value
			cond, s = s.PopVal()
			enabled = truthy(cond)
		}
		if n.Flags&HasChoiceOnlyContent != 0 {
			var x StringValue
			x, s = pop[StringValue](s)
			label = x
		}
		if n.Flags&HasStartContent != 0 {
			var x StringValue
			x, s = pop[StringValue](s)
			label = x + label
		}
		if n.Flags&OnceOnly != 0 {
			addr, _ := el.Find(n.Dest).Address()
			visits := e.Stack.VisitCount(addr)
			if visits != 0 {
				enabled = false
			}
		}
		// TODO error here if we can't find the target for this choice?
		var choice *Choice
		if enabled {
			isInvisibleDefault := n.Flags&IsInvisibleDefault != 0
			dest := Divert{Dest: n.Dest, incTurnCount: !isInvisibleDefault}
			choice = &Choice{
				Label:              string(label),
				Dest:               choiceElement{node: dest, src: el},
				IsInvisibleDefault: isInvisibleDefault,
			}
		}
		s, next := popIfEnded(s, el.Next())
		return "", choice, next, endEval(s)
	case SetVar:
		val, s := e.Stack.PopVal()
		if n.Reassign {
			s = s.UpdateVar(n.Name, val)
		} else {
			s = s.WithGlobal(n.Name, val)
		}
		s, next := popIfEnded(s, el.Next())
		return "", nil, next, endEval(s)
	case FuncReturn:
		s, ret := e.Stack.PopFrame()
		return "", nil, ret, endEval(s)
	case TunnelCall:
		s := e.Stack.PushFrame(el.Next())
		dest := el.Find(n.Dest)
		if dest == nil {
			panic(fmt.Errorf("tunnel call target %q not found", n.Dest))
		}
		return "", nil, dest, BaseEvaluator{Stack: s}
	case TunnelReturn:
		rv, s := e.Stack.PopVal()
		s, ret := s.PopFrame()
		if ret == nil {
			panic(fmt.Errorf("Found tunnel onwards ->-> but no tunnel to return to"))
		}
		switch rv := rv.(type) {
		case VoidValue:
			return "", nil, ret, endEval(s)
		case DivertTargetValue:
			return "", nil, el.Find(rv.Dest), endEval(s)
		default:
			panic(fmt.Errorf("unexpected tunnel return value %T", rv))
		}
	case NoOp:
		s, next := popIfEnded(e.Stack, el.Next())
		return "", nil, next, endEval(s)
	case Done, End:
		return "", nil, nil, e
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}

type choiceElement struct {
	node Node
	src  Element
}

func (e choiceElement) Node() Node {
	return e.node
}

func (e choiceElement) Address() (Address, int) {
	return e.src.Address()
}

func (e choiceElement) Find(addr Address) Element {
	return e.src.Find(addr)
}

func (e choiceElement) Next() Element {
	panic("should have followed the Divert")
}

func pop[T any](s *CallFrame) (T, *CallFrame) {
	val, s := s.PopVal()
	return val.(T), s
}

type EvalEvaluator struct {
	Stack *CallFrame
}

func endEval(s *CallFrame) Evaluator {
	switch {
	case s.stringMode:
		return StringEvaluator{Stack: s}
	case s.evalDepth > 0:
		return EvalEvaluator{Stack: s}
	default:
		return BaseEvaluator{Stack: s}
	}
}

func (e EvalEvaluator) endEval() Evaluator {
	return endEval(e.Stack.IncEvalDepth(-1))
}

func (e EvalEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
	e.Stack = e.Stack.Visit(el.Address())
	switch n := el.Node().(type) {
	case BeginStringEval:
		s := e.Stack.WithStringMode(true)
		return "", nil, el.Next(), StringEvaluator{Stack: s}
	case EndEval:
		return "", nil, el.Next(), e.endEval()
	case GetVar:
		val, ok := e.Stack.GetVar(n.Name)
		if !ok {
			panic(fmt.Errorf("variable %q not found", n.Name))
		}
		s := e.Stack.PushVal(val)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case SetVar:
		val, s := e.Stack.PopVal()
		if n.Reassign {
			s = s.UpdateVar(n.Name, val)
		} else {
			s = s.WithGlobal(n.Name, val)
		}
		s, next := popIfEnded(s, el.Next())
		return "", nil, next, endEval(s)
	case SetTemp:
		val, s := e.Stack.PopVal()
		// TODO use "re" reassign flag to check that it's already
		// been set?
		s = s.WithLocal(n.Name, val)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case DivertTargetValue, IntValue, FloatValue, BoolValue, ListValue, Text:
		s := e.Stack.PushVal(n)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case BinOp:
		b, s := e.Stack.PopVal()
		a, s := s.PopVal()
		s = s.PushVal(n(a, b))
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case UnaryOp:
		a, s := e.Stack.PopVal()
		s = s.PushVal(n(a))
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case Pop:
		_, s := e.Stack.PopVal()
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case Divert:
		addr := n.Dest
		if n.Var {
			addrVar, ok := e.Stack.GetVar(string(addr))
			if !ok {
				panic(fmt.Errorf("address variable %q not found", addr))
			}
			addr = addrVar.(DivertTargetValue).Dest
		}
		if n.Conditional {
			var cond Value
			cond, e.Stack = e.Stack.PopVal()
			if !truthy(cond) {
				return "", nil, el.Next(), e
			}
		}
		if n.incTurnCount {
			e.Stack = e.Stack.IncTurnCount()
		}
		dest := el.Find(addr)
		if dest == nil {
			panic(fmt.Errorf("divert target %q not found", n.Dest))
		}
		return "", nil, dest, e
	case FuncCall:
		s := e.Stack.PushFrame(el.Next())
		dest := el.Find(n.Dest)
		if dest == nil {
			panic(fmt.Errorf("function call target %q not found", n.Dest))
		}
		return "", nil, dest, BaseEvaluator{Stack: s}
	case TurnCounter:
		turn := IntValue(e.Stack.turnCount)
		s := e.Stack.PushVal(turn)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case GetVisitCount:
		base, _ := el.Address()
		addr := resolve(base, Address(n.Container))
		count := IntValue(e.Stack.VisitCount(addr))
		s := e.Stack.PushVal(count)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case Done, End:
		return "", nil, nil, BaseEvaluator{Stack: e.Stack}
	case Out:
		val, s := e.Stack.PopVal()
		o := val.(Outputter).Output()
		s, newline := s.ShouldPrependNewline()
		if newline {
			o = "\n" + o
		}
		return o, nil, el.Next(), EvalEvaluator{Stack: s}
	case Void:
		s := e.Stack.PushVal(VoidValue{})
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case VarRef:
		s := e.Stack.PushVarRef(n.Name)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListInt:
		val, s := e.Stack.PopVal()
		origin, s := s.PopVal()
		v := s.ListInt(string(origin.(Text)), int(val.(IntValue)))
		s = s.PushVal(v)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListValueFunc:
		val, s := pop[ListValue](e.Stack)
		if len(val.Items) == 0 {
			s = s.PushVal(IntValue(0))
		} else {
			s = s.PushVal(IntValue(val.Items[len(val.Items)-1].Value))
		}
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListCountFunc:
		val, s := pop[ListValue](e.Stack)
		count := IntValue(len(val.Items))
		s = s.PushVal(count)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListMinFunc:
		val, s := pop[ListValue](e.Stack)
		if len(val.Items) == 0 {
			s = s.PushVal(val)
		} else {
			s = s.PushVal(val.At(0))
		}
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListMaxFunc:
		val, s := pop[ListValue](e.Stack)
		if len(val.Items) == 0 {
			s = s.PushVal(val)
		} else {
			s = s.PushVal(val.At(len(val.Items) - 1))
		}
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListAllFunc:
		val, s := pop[ListValue](e.Stack)
		v := s.ListAll(val)
		s = s.PushVal(v)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListInvertFunc:
		val, s := pop[ListValue](e.Stack)
		v := s.ListAll(val)
		s = s.PushVal(v.Sub(val))
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListRangeFunc:
		end, s := pop[IntValue](e.Stack)
		start, s := pop[IntValue](s)
		val, s := pop[ListValue](s)
		s = s.PushVal(val.Range(int(start), int(end)))
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case ListIntersectFunc:
		a, s := pop[ListValue](e.Stack)
		b, s := pop[ListValue](s)
		s = s.PushVal(a.Intersect(b))
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}

func resolve(base, addr Address) Address {
	after, ok := strings.CutPrefix(string(addr), ".^")
	if !ok {
		return addr
	}
	if after == "" {
		return base
	}
	return resolve(base.Parent(), Address(after))
}

type StringEvaluator struct {
	Stack   *CallFrame
	wrapped Evaluator
	output  string
}

func (e StringEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
	e.Stack = e.Stack.Visit(el.Address())
	switch n := el.Node().(type) {
	case Text:
		return "", nil, el.Next(), e.pushText(string(n))
	case NoOp:
		return "", nil, el.Next(), e
	case BeginEval:
		s := e.Stack.IncEvalDepth(1)
		return "", nil, el.Next(), StringWrappedEvaluator{
			output:  e.output,
			wrapped: EvalEvaluator{Stack: s},
		}
	case Divert:
		addr := n.Dest
		if n.Var {
			addrVar, ok := e.Stack.GetVar(string(addr))
			if !ok {
				panic(fmt.Errorf("address variable %q not found", addr))
			}
			addr = addrVar.(DivertTargetValue).Dest
		}
		if n.Conditional {
			var cond IntValue
			cond, e.Stack = pop[IntValue](e.Stack)
			if cond == 0 {
				return "", nil, el.Next(), e
			}
		}
		dest := el.Find(addr)
		if dest == nil {
			panic(fmt.Errorf("divert target %q not found", n.Dest))
		}
		return "", nil, dest, e
	case EndStringEval:
		s := e.Stack.PushVal(StringValue(e.output))
		s = s.WithStringMode(false)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}

func (e StringEvaluator) pushText(s string) StringEvaluator {
	return StringEvaluator{
		Stack:  e.Stack,
		output: e.output + s,
	}
}

type StringWrappedEvaluator struct {
	wrapped Evaluator
	output  string
}

func (e StringWrappedEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
	out, choice, next, eval := e.wrapped.Step(el)
	if out.String() != "" {
		e.output += out.String()
	}
	if se, ok := eval.(StringEvaluator); ok {
		return "", choice, next, StringEvaluator{
			Stack:  se.Stack,
			output: e.output,
		}
	}
	return "", choice, next, StringWrappedEvaluator{
		wrapped: eval,
		output:  e.output,
	}
}

type TagEvaluator struct {
	Stack *CallFrame
}

func (e TagEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
	switch n := el.Node().(type) {
	case Text:
		// TODO store tags on the previous output?
		// or maybe buffer tags until we reach a Newline{}
		return "", nil, el.Next(), e
	case EndTag:
		return "", nil, el.Next(), BaseEvaluator{Stack: e.Stack}
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}
