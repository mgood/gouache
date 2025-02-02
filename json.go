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
	if err := json.NewDecoder(r).Decode(&b); err != nil {
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
				c.Flags = ContainerFlag(v.(float64))
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
		}
		if s := strings.TrimPrefix(n, "^"); s != n {
			return Text(s)
		}
		panic(fmt.Errorf("unsupported node: %q", n))
	case map[string]any:
		if v, ok := n["*"]; ok {
			return ChoicePoint{
				Dest:  Address(v.(string)),
				Flags: ChoicePointFlag(n["flg"].(float64)),
			}
		}
		if v, ok := n["->"]; ok {
			r := Divert{
				Dest: Address(v.(string)),
			}
			if v, ok := n["var"]; ok {
				r.Var = v.(bool)
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
		panic(fmt.Errorf("unsupported node: %#v", n))
	case []any:
		return LoadContainer(n)
	}
	panic(fmt.Errorf("unsupported node: %#v", n))
}
