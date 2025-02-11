package gouache

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var ErrUnsupportedVersion = fmt.Errorf("unsupported version")

func LoadJSON(r io.Reader) (Element, error) {
	var b struct {
		Version int   `json:"inkVersion"`
		Root    []any `json:"root"`
	}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&b); err != nil {
		return nil, err
	}
	if b.Version != InkVersion {
		return nil, ErrUnsupportedVersion
	}
	return LoadContainer(b.Root).First(), nil
}

func LoadContainer(contents []any) Container {
	var c Container
	meta := contents[len(contents)-1]
	if meta != nil {
		for k, v := range meta.(map[string]any) {
			switch k {
			case "#n":
				c.Name = v.(string)
			case "#f":
				f, err := v.(json.Number).Float64()
				if err != nil {
					panic(err)
				}
				c.Flags = ContainerFlag(f)
			default:
				n := LoadContainer(v.([]any))
				n.Name = k
				if c.Nested == nil {
					c.Nested = make(map[string]Container)
				}
				c.Nested[k] = n
			}
		}
	}
	c.Contents = make([]Node, 0, len(contents)-1)
	for _, n := range contents[:len(contents)-1] {
		c.Contents = append(c.Contents, loadNode(n))
	}
	return c
}

func loadNode(n any) Node {
	switch n := n.(type) {
	case json.Number:
		if s := n.String(); strings.Contains(s, ".") {
			f, err := n.Float64()
			if err != nil {
				panic(err)
			}
			return FloatValue(f)
		}
		i, err := n.Int64()
		if err != nil {
			panic(err)
		}
		return IntValue(i)
	case bool:
		return boolean(n)
	case string:
		switch n {
		case "done":
			return Done{}
		case "\n":
			return Newline{}
		case "ev":
			return BeginEval{}
		case "/ev":
			return EndEval{}
		case "str":
			return BeginStringEval{}
		case "/str":
			return EndStringEval{}
		case "#":
			return BeginTag{}
		case "/#":
			return EndTag{}
		case "out":
			return Out{}
		case "pop":
			return Pop{}
		case "end":
			return End{}
		case "nop":
			return NoOp{}
		case "void":
			return Void{}
		case "turn":
			return TurnCounter{}
		case "~ret":
			return FuncReturn{}
		case "+":
			return Add
		case "-":
			return Sub
		case "/":
			return Div
		case "*":
			return Mul
		case "%":
			return Mod
		case "_":
			return Neg
		case "&&":
			return And
		case "||":
			return Or
		case "==":
			return Eq
		case "!=":
			return Ne
		case "<":
			return Lt
		case "<=":
			return Lte
		case ">":
			return Gt
		case ">=":
			return Gte
		case "!":
			return Not
		case "MIN":
			return Min
		case "MAX":
			return Max
		}
		if s, found := strings.CutPrefix(n, "^"); found {
			return Text(s)
		}
		panic(fmt.Errorf("unsupported node: %q", n))
	case map[string]any:
		if v, ok := n["*"]; ok {
			flg, err := n["flg"].(json.Number).Int64()
			if err != nil {
				panic(err)
			}
			return ChoicePoint{
				Dest:  Address(v.(string)),
				Flags: ChoicePointFlag(flg),
			}
		}
		if v, ok := n["->"]; ok {
			r := Divert{
				Dest: Address(v.(string)),
			}
			if v, ok := n["var"]; ok {
				r.Var = v.(bool)
			}
			if c, ok := n["c"]; ok {
				r.Conditional = c.(bool)
			}
			return r
		}
		if v, ok := n["^->"]; ok {
			return DivertTargetValue{
				Dest: Address(v.(string)),
			}
		}
		if v, ok := n["temp="]; ok {
			return SetTemp{
				Name: v.(string),
			}
		}
		if v, ok := n["VAR="]; ok {
			return SetVar{
				Name: v.(string),
			}
		}
		if v, ok := n["VAR?"]; ok {
			return GetVar{
				Name: v.(string),
			}
		}
		if v, ok := n["CNT?"]; ok {
			return GetVisitCount{
				Container: v.(string),
			}
		}
		if v, ok := n["f()"]; ok {
			return FuncCall{
				Dest: Address(v.(string)),
			}
		}
		if v, ok := n["^var"]; ok {
			ci, err := n["ci"].(json.Number).Int64()
			if err != nil {
				panic(err)
			}
			return VarRef{
				Name:         v.(string),
				ContentIndex: int(ci),
			}
		}
		panic(fmt.Errorf("unsupported node: %#v", n))
	case []any:
		return LoadContainer(n)
	}
	panic(fmt.Errorf("unsupported node: %#v", n))
}
