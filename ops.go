package gouache

import "fmt"

func (n Divert) GetDest(el Element, stack *CallFrame) (Element, *CallFrame) {
	addr := n.Dest
	if n.Var {
		addrVar, ok := stack.GetVar(string(addr))
		if !ok {
			panic(fmt.Errorf("address variable %q not found", addr))
		}
		addr = addrVar.(DivertTargetValue).Dest
	}
	if n.Conditional {
		var cond Value
		cond, stack = stack.PopVal()
		if !truthy(cond) {
			return el.Next(), stack
		}
	}
	dest := el.Find(addr)
	if dest == nil {
		panic(fmt.Errorf("divert target %q not found", n.Dest))
	}
	return dest, stack
}

func (n DupTop) Apply(stack *CallFrame) *CallFrame {
	v, stack := stack.PopVal()
	stack = stack.PushVal(v)
	return stack.PushVal(v)
}

func (n GetVar) Apply(stack *CallFrame) *CallFrame {
	val, ok := stack.GetVar(n.Name)
	if !ok {
		panic(fmt.Errorf("variable %q not found", n.Name))
	}
	return stack.PushVal(val)
}

func (n SetVar) Apply(stack *CallFrame) *CallFrame {
	var val Value
	val, stack = stack.PopVal()
	if n.Reassign {
		return stack.UpdateVar(n.Name, val)
	}
	return stack.WithGlobal(n.Name, val)
}

func (n SetTemp) Apply(stack *CallFrame) *CallFrame {
	val, stack := stack.PopVal()
	if n.Reassign {
		return stack.WithLocal(n.Name, val)
	}
	return stack.DeclareLocal(n.Name, val)
}
