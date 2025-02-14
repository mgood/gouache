package gouache

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var ErrUnsupportedVersion = fmt.Errorf("unsupported version")

func LoadJSON(r io.Reader) (Element, ListDefs, error) {
	var b struct {
		Version  int      `json:"inkVersion"`
		Root     []any    `json:"root"`
		ListDefs ListDefs `json:"listDefs"`
	}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&b); err != nil {
		return nil, nil, err
	}
	if b.Version != InkVersion {
		return nil, nil, ErrUnsupportedVersion
	}
	return LoadContainer(b.Root).First(), b.ListDefs, nil
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
		case "du":
			return DupTop{}
		case "end":
			return End{}
		case "nop":
			return NoOp{}
		case "void":
			return Void{}
		case "turn":
			return TurnCounter{}
		case "turns":
			return TurnsSince{}
		case "visit":
			return VisitCounter{}
		case "~ret":
			return FuncReturn{}
		case "->->":
			return TunnelReturn{}
		case "thread":
			return ThreadStart{}
		case "listInt":
			return ListInt{}
		case "LIST_VALUE":
			return ListValueFunc{}
		case "LIST_COUNT":
			return ListCountFunc{}
		case "LIST_MIN":
			return ListMinFunc{}
		case "LIST_MAX":
			return ListMaxFunc{}
		case "LIST_ALL":
			return ListAllFunc{}
		case "LIST_INVERT":
			return ListInvertFunc{}
		case "L^":
			return ListIntersectFunc{}
		case "range":
			return ListRangeFunc{}
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
		case "?":
			return Has
		case "!?":
			return Hasnt
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
			r := SetVar{
				Name: v.(string),
			}
			if v, ok := n["re"]; ok {
				r.Reassign = v.(bool)
			}
			return r
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
		if v, ok := n["->t->"]; ok {
			return TunnelCall{
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
		if v, ok := n["list"]; ok {
			list := ListValue{
				Origins: make(map[string]struct{}),
			}
			for k, vv := range v.(map[string]any) {
				i, err := vv.(json.Number).Int64()
				if err != nil {
					panic(err)
				}
				origin, name, ok := strings.Cut(k, ".")
				if !ok {
					panic(fmt.Errorf("unsupported list item: %q", k))
				}
				list = list.Put(origin, name, int(i))
			}
			if origins := n["origins"]; origins != nil {
				for _, origin := range origins.([]any) {
					list.Origins[origin.(string)] = struct{}{}
				}
			}
			return list
		}
		panic(fmt.Errorf("unsupported node: %#v", n))
	case []any:
		return LoadContainer(n)
	}
	panic(fmt.Errorf("unsupported node: %#v", n))
}
