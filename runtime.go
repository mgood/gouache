package gouache

import (
	"fmt"
	"strings"

	"github.com/mgood/gouache/glue"
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
	Eval               Evaluator
	IsInvisibleDefault bool
}

type Element interface {
	Node() Node
	Address() (Address, int)
	Find(Address) (Element, []VisitAddr)
	Next() (Element, []VisitAddr)
}

type Evaluator interface {
	Step(Element) (Output, *Choice, Element, Evaluator)
}

type StepEvaluator struct {
	Stack   *CallFrame
	Stepper Stepper
}

func (e StepEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
	switch el.Node().(type) {
	case End:
		// FIXME end is supposed to unwind the full stack
		return "", nil, nil, StepEvaluator{Stack: e.Stack, Stepper: BaseEvaluator{}}
	}
	stack := e.Stack
	out, choice, elem, stack, stepper := e.Stepper.Step(stack, el)
	if choice != nil {
		choiceStack := stack.ResetChoiceCount()
		if !choice.IsInvisibleDefault {
			choiceStack = choiceStack.IncTurnCount()
		}
		choice.Eval = StepEvaluator{Stack: choiceStack, Stepper: stepper}
	}
	if elem == nil {
		var nextStepper Stepper
		var isFunction bool
		stack, elem, nextStepper, isFunction = stack.PopFrame()
		if nextStepper == nil {
			nextStepper = BaseEvaluator{}
		} else if sw, ok := stepper.(StringWrappedEvaluator); ok {
			// If we were capturing the output, restore capturing
			// of the output to the previous frame.
			// Maybe the output capture should go into the stack instead?
			sw.wrapped = nextStepper
			if isFunction {
				sw.output += string(glue.FuncEnd)
			}
			stepper = sw
		} else {
			stepper = nextStepper
			if isFunction {
				out += Output(glue.FuncEnd)
			}
		}
		stack = stack.PushVal(VoidValue{})
		if elem != nil {
			elem, stack = visitNext(elem, stack)
		}
	}
	return out, choice, elem, StepEvaluator{Stack: stack, Stepper: stepper}
}

type Stepper interface {
	Step(*CallFrame, Element) (Output, *Choice, Element, *CallFrame, Stepper)
}

func Init(c Container, listDefs ListDefs) (Element, Evaluator) {
	var eval Evaluator = StepEvaluator{
		Stack: &CallFrame{
			listDefs: listDefs,
		},
		Stepper: BaseEvaluator{},
	}
	if g, _ := c.Find("global decl"); g != nil {
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
	// FIXME we should be able to initialize the visit state from any starting
	// element but for now assume we're going to start at the root
	root, visitAddrs := c.atNoFlatten(0).Flatten()
	se := eval.(StepEvaluator)
	se.Stack = visit("", visitAddrs, se.Stack)
	return root, se
}

func Continue(output glue.StringWriter, eval Evaluator, elem Element) []Choice {
	var choices []Choice
	var defaultChoice *Choice
	var s Output
	var choice *Choice
	for ; ; s, choice, elem, eval = eval.Step(elem) {
		output.WriteString(s.String())
		if choice != nil {
			if choice.IsInvisibleDefault {
				defaultChoice = choice
			} else {
				choices = append(choices, *choice)
			}
		}
		if elem != nil {
			continue
		}
		if len(choices) == 0 && defaultChoice != nil {
			elem = defaultChoice.Dest
			eval = defaultChoice.Eval
			defaultChoice = nil
			continue
		}
		break
	}
	return choices
}

// elements should report their path
// track number of elements visited in this parent
// when parent changes, record the visits for the container
// though follow the flags on the parent to determine when to update
// RecordVisits
// CountTurns
// CountStartOnly

type BaseEvaluator struct {
}

func visitNext(el Element, stack *CallFrame) (Element, *CallFrame) {
	from, _ := el.Address()
	next, addrs := el.Next()
	return next, visit(from, addrs, stack)
}

func visit(from Address, addrs []VisitAddr, stack *CallFrame) *CallFrame {
	for _, addr := range addrs {
		stack = stack.Visit(addr, from)
		from = addr.Addr
	}
	return stack
}

func (e BaseEvaluator) Step(stack *CallFrame, el Element) (Output, *Choice, Element, *CallFrame, Stepper) {
	switch n := el.Node().(type) {
	case Text:
		next, stack := visitNext(el, stack)
		return Output(n), nil, next, stack, e
	case Newline:
		o := Output("\n")
		next, stack := visitNext(el, stack)
		return o, nil, next, stack, e
	case Glue:
		next, stack := visitNext(el, stack)
		return Output(glue.Glue), nil, next, stack, e
	case BeginEval:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, EvalEvaluator{Prev: e}
	case SetTemp:
		stack = n.Apply(stack)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Pop:
		_, stack = stack.PopVal()
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case DupTop:
		stack = n.Apply(stack)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Divert:
		dest, stack := n.GetDest(el, stack)
		return "", nil, dest, stack, e
	case BeginTag:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, TagEvaluator{Prev: e}
	case ChoicePoint:
		var label StringValue
		enabled := true
		if n.Flags&HasCondition != 0 {
			var cond Value
			cond, stack = stack.PopVal()
			enabled = truthy(cond)
		}
		if n.Flags&HasChoiceOnlyContent != 0 {
			var x StringValue
			x, stack = pop[StringValue](stack)
			label = x
		}
		if n.Flags&HasStartContent != 0 {
			var x StringValue
			x, stack = pop[StringValue](stack)
			label = x + label
		}
		if n.Flags&OnceOnly != 0 {
			dest, _ := el.Find(n.Dest)
			addr, _ := dest.Address()
			visits := stack.VisitCount(addr)
			if visits != 0 {
				enabled = false
			}
		}
		if !enabled {
			next, stack := visitNext(el, stack)
			return "", nil, next, stack, e
		}
		isInvisibleDefault := n.Flags&IsInvisibleDefault != 0
		dest := Divert{
			Dest: n.Dest,
		}
		choice := &Choice{
			Label:              string(label),
			Dest:               choiceElement{node: dest, src: el},
			IsInvisibleDefault: isInvisibleDefault,
		}
		stack = stack.IncChoiceCount()
		next, stack := visitNext(el, stack)
		return "", choice, next, stack, e
	case SetVar:
		val, stack := stack.PopVal()
		if n.Reassign {
			stack = stack.UpdateVar(n.Name, val)
		} else {
			stack = stack.WithGlobal(n.Name, val)
		}
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case FuncReturn:
		stack, ret, eval, isFunction := stack.PopFrame()
		if !isFunction {
			panic(fmt.Errorf("unexpected function return"))
		}
		ret, stack = visitNext(ret, stack)
		return Output(glue.FuncEnd), nil, ret, stack, eval
	case TunnelCall:
		addr := n.Dest
		if n.Var {
			addrVar, ok := stack.GetVar(string(addr))
			if !ok {
				panic(fmt.Errorf("address variable %q not found", addr))
			}
			addr = addrVar.(DivertTargetValue).Dest
		}
		dest, visitAddr := el.Find(addr)
		if dest == nil {
			panic(fmt.Errorf("tunnel call target %q not found", n.Dest))
		}
		stack = stack.PushFrame(el, e, false)
		from, _ := el.Address()
		stack = visit(from, visitAddr, stack)
		return "", nil, dest, stack, e
	case ThreadStart:
		next, stack := visitNext(el, stack)
		stack = stack.PushFrame(next, e, false)
		return "", nil, next, stack, e
	case TunnelReturn:
		rv, stack := stack.PopVal()
		stack, ret, eval, isFunction := stack.PopFrame()
		if isFunction {
			panic(fmt.Errorf("unexpected tunnel return in function"))
		}
		if ret == nil {
			panic(fmt.Errorf("Found tunnel onwards ->-> but no tunnel to return to"))
		}
		switch rv := rv.(type) {
		case VoidValue:
			ret, stack = visitNext(ret, stack)
		case DivertTargetValue:
			ret, _ = el.Find(rv.Dest)
		default:
			panic(fmt.Errorf("unexpected tunnel return value %T", rv))
		}
		return "", nil, ret, stack, eval
	case NoOp:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case IntValue, FloatValue:
		// raw int and float outside of an eval block are ignored
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Out:
		val, stack := stack.PopVal()
		o := val.(Outputter).Output()
		next, stack := visitNext(el, stack)
		return o, nil, next, stack, e
	case Done:
		return "", nil, nil, stack, e
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

func (e choiceElement) Find(addr Address) (Element, []VisitAddr) {
	return e.src.Find(addr)
}

func (e choiceElement) Next() (Element, []VisitAddr) {
	panic("should have followed the Divert")
}

func pop[T any](s *CallFrame) (T, *CallFrame) {
	val, s := s.PopVal()
	return val.(T), s
}

type EvalEvaluator struct {
	Prev Stepper
}

func (e EvalEvaluator) Step(stack *CallFrame, el Element) (Output, *Choice, Element, *CallFrame, Stepper) {
	switch n := el.Node().(type) {
	case BeginStringEval:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, StringEvaluator{Prev: e}
	case EndEval:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e.Prev
	case GetVar:
		val, ok := stack.GetVar(n.Name)
		if !ok {
			panic(fmt.Errorf("variable %q not found", n.Name))
		}
		stack = stack.PushVal(val)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case SetVar:
		stack = n.Apply(stack)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case SetTemp:
		stack = n.Apply(stack)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case DivertTargetValue, IntValue, FloatValue, BoolValue, ListValue:
		stack = stack.PushVal(n)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Text:
		stack = stack.PushVal(StringValue(n))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case BinOp:
		b, stack := stack.PopVal()
		a, stack := stack.PopVal()
		stack = stack.PushVal(n(a, b))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case UnaryOp:
		a, stack := stack.PopVal()
		stack = stack.PushVal(n(a))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Pop:
		_, stack = stack.PopVal()
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Divert:
		dest, stack := n.GetDest(el, stack)
		return "", nil, dest, stack, e
	case FuncCall:
		addr := n.Dest
		if n.Var {
			addrVar, ok := stack.GetVar(string(addr))
			if !ok {
				panic(fmt.Errorf("address variable %q not found", addr))
			}
			addr = addrVar.(DivertTargetValue).Dest
		}
		dest, visitAddrs := el.Find(addr)
		if dest == nil {
			panic(fmt.Errorf("function call target %q not found", n.Dest))
		}
		from, _ := el.Address()
		stack = visit(from, visitAddrs, stack)
		stack = stack.PushFrame(el, e, true)
		return Output(glue.FuncStart), nil, dest, stack, BaseEvaluator{}
	case TurnCounter:
		turn := IntValue(stack.turnCount)
		stack = stack.PushVal(turn)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case GetVisitCount:
		base, _ := el.Address()
		addr := resolve(base, Address(n.Container))
		count := IntValue(stack.VisitCount(addr))
		stack = stack.PushVal(count)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case VisitIndex:
		base, _ := el.Address()
		addr := base
		count := IntValue(stack.VisitCount(addr))
		// here we want 0-indexed for the current container, so subtract 1
		stack = stack.PushVal(count - 1)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ReadCountFunc:
		target, stack := pop[DivertTargetValue](stack)
		base, _ := el.Address()
		addr := resolve(base, Address(target.Dest))
		count := IntValue(stack.VisitCount(addr))
		stack = stack.PushVal(count)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case TurnsSince:
		dv, stack := pop[DivertTargetValue](stack)
		base, _ := el.Address()
		addr := resolve(base, dv.Dest)
		count := IntValue(stack.TurnsSince(addr))
		stack = stack.PushVal(count)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Out:
		val, stack := stack.PopVal()
		o := val.(Outputter).Output()
		next, stack := visitNext(el, stack)
		return o, nil, next, stack, e
	case Void:
		stack = stack.PushVal(VoidValue{})
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case VarRef:
		stack = stack.PushVarRef(n.Name)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListInt:
		val, stack := stack.PopVal()
		origin, stack := stack.PopVal()
		v := stack.ListInt(string(origin.(StringValue)), int(val.(IntValue)))
		stack = stack.PushVal(v)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListValueFunc:
		val, stack := pop[ListValue](stack)
		if len(val.Items) == 0 {
			stack = stack.PushVal(IntValue(0))
		} else {
			stack = stack.PushVal(IntValue(val.Items[len(val.Items)-1].Value))
		}
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListCountFunc:
		val, stack := pop[ListValue](stack)
		count := IntValue(len(val.Items))
		stack = stack.PushVal(count)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListMinFunc:
		val, stack := pop[ListValue](stack)
		if len(val.Items) == 0 {
			stack = stack.PushVal(val)
		} else {
			stack = stack.PushVal(val.At(0))
		}
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListMaxFunc:
		val, stack := pop[ListValue](stack)
		if len(val.Items) == 0 {
			stack = stack.PushVal(val)
		} else {
			stack = stack.PushVal(val.At(len(val.Items) - 1))
		}
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListAllFunc:
		val, stack := pop[ListValue](stack)
		v := stack.ListAll(val)
		stack = stack.PushVal(v)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListInvertFunc:
		val, stack := pop[ListValue](stack)
		v := stack.ListAll(val)
		stack = stack.PushVal(v.Sub(val))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListRangeFunc:
		end, stack := stack.PopVal()
		start, stack := stack.PopVal()
		val, stack := pop[ListValue](stack)
		stack = stack.PushVal(val.Range(start, end))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ListIntersectFunc:
		a, stack := pop[ListValue](stack)
		b, stack := pop[ListValue](stack)
		stack = stack.PushVal(a.Intersect(b))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case DupTop:
		stack = n.Apply(stack)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case ChoiceCounter:
		count := IntValue(stack.ChoiceCount())
		stack = stack.PushVal(count)
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case Seq:
		elements, stack := pop[IntValue](stack)
		seqCount, stack := pop[IntValue](stack)
		addr, _ := el.Address()
		index := shuffle(string(addr), int(elements), int(seqCount))
		stack = stack.PushVal(IntValue(index))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
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
	output string
	Prev   Stepper
}

func (e StringEvaluator) Step(stack *CallFrame, el Element) (Output, *Choice, Element, *CallFrame, Stepper) {
	switch n := el.Node().(type) {
	case Text:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e.pushText(string(n))
	case NoOp:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case BeginEval:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, StringWrappedEvaluator{
			output:  e.output,
			wrapped: EvalEvaluator{Prev: e},
		}
	case Divert:
		dest, stack := n.GetDest(el, stack)
		return "", nil, dest, stack, e
	case EndStringEval:
		stack = stack.PushVal(StringValue(glue.StripInline(e.output)))
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e.Prev
	case BeginTag:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, TagEvaluator{Prev: e}
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}

func (e StringEvaluator) pushText(s string) StringEvaluator {
	e.output += s
	return e
}

type StringWrappedEvaluator struct {
	wrapped Stepper
	output  string
	depth   int
}

func (e StringWrappedEvaluator) Step(stack *CallFrame, el Element) (Output, *Choice, Element, *CallFrame, Stepper) {
	switch el.Node().(type) {
	case BeginEval:
		e.depth++
	case EndEval:
		e.depth--
	}
	out, choice, next, stack, eval := e.wrapped.Step(stack, el)
	if s := out.String(); s != "" {
		e.output += s
	}
	if e.depth < 0 {
		// once eval stack ends, we expect to be back at the prior string evaluator
		// so we set its accumulated output and then return there
		streval := eval.(StringEvaluator)
		streval.output = e.output
		eval = streval
	} else {
		e.wrapped = eval
		eval = e
	}
	return "", choice, next, stack, eval
}

type TagEvaluator struct {
	Prev Stepper
}

func (e TagEvaluator) Step(stack *CallFrame, el Element) (Output, *Choice, Element, *CallFrame, Stepper) {
	switch n := el.Node().(type) {
	case Text:
		// TODO store tags on the previous output?
		// or maybe buffer tags until we reach a Newline{}
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e
	case EndTag:
		next, stack := visitNext(el, stack)
		return "", nil, next, stack, e.Prev
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}
