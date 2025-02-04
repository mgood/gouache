package gouache

import "fmt"

type Output string

func (o Output) String() string {
	return string(o)
}

type Outputter interface {
	Output() Output
}

type Choice struct {
	Label string
	Dest  Element
}

type Element interface {
	Node() Node
	Find(Address) Element
	Next() Element
}

type Evaluator interface {
	Step(Element) (Output, *Choice, Element, Evaluator)
}

func Init(root Element) Evaluator {
	var eval Evaluator = BaseEvaluator{}
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

type BaseEvaluator struct {
	Stack *CallFrame
}

func (e BaseEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
	switch n := el.Node().(type) {
	case Text:
		return Output(n), nil, el.Next(), e
	case Newline:
		return Output("\n"), nil, el.Next(), e
	case BeginEval:
		s := e.Stack.IncEvalDepth(1)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case SetTemp:
		val, s := e.Stack.PopVal()
		s = s.WithLocal(n.Name, val)
		return "", nil, el.Next(), BaseEvaluator{Stack: s}
	case Pop:
		_, s := e.Stack.PopVal()
		return "", nil, el.Next(), BaseEvaluator{Stack: s}
	case Divert:
		addr := n.Dest
		if n.Var {
			addrVar, ok := e.Stack.GetVar(string(addr))
			if !ok {
				panic(fmt.Errorf("address variable %q not found", addr))
			}
			addr = addrVar.(DivertTargetValue).Dest
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
		s := e.Stack
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
		// TODO error if we can't find the target for this choice
		choice := &Choice{string(label), el.Find(n.Dest)}
		return "", choice, el.Next(), BaseEvaluator{Stack: s}
	case SetVar:
		val, s := e.Stack.PopVal()
		s = s.WithGlobal(n.Name, val)
		return "", nil, el.Next(), BaseEvaluator{Stack: s}
	case Done, End:
		return "", nil, nil, e
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}

func pop[T any](s *CallFrame) (T, *CallFrame) {
	val, s := s.PopVal()
	return val.(T), s
}

type EvalEvaluator struct {
	Stack *CallFrame
}

func (e EvalEvaluator) endEval() Evaluator {
	s := e.Stack.IncEvalDepth(-1)
	switch {
	case s.stringMode:
		return StringEvaluator{Stack: s}
	case s.evalDepth > 0:
		return EvalEvaluator{Stack: s}
	default:
		return BaseEvaluator{Stack: s}
	}
}

func (e EvalEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
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
		s = s.WithGlobal(n.Name, val)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case SetTemp:
		val, s := e.Stack.PopVal()
		s = s.WithLocal(n.Name, val)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case DivertTargetValue, IntValue:
		s := e.Stack.PushVal(n)
		return "", nil, el.Next(), EvalEvaluator{Stack: s}
	case BinOp:
		b, s := e.Stack.PopVal()
		a, s := s.PopVal()
		s = s.PushVal(n(a, b))
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
		dest := el.Find(addr)
		if dest == nil {
			panic(fmt.Errorf("divert target %q not found", n.Dest))
		}
		return "", nil, dest, e
	case Done, End:
		return "", nil, nil, BaseEvaluator{Stack: e.Stack}
	case Out:
		val, s := e.Stack.PopVal()
		o := val.(Outputter).Output()
		return o, nil, el.Next(), EvalEvaluator{Stack: s}
	default:
		panic(fmt.Errorf("unexpected node type %T", n))
	}
}

type StringEvaluator struct {
	Stack   *CallFrame
	wrapped Evaluator
	output  string
}

func (e StringEvaluator) Step(el Element) (Output, *Choice, Element, Evaluator) {
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
