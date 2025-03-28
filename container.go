package gouache

import (
	"fmt"
	"strconv"
	"strings"
)

type ContainerElement struct {
	Self  Container
	Index int
}

var _ Element = ContainerElement{}

func (e ContainerElement) Find(name Address) (Element, []VisitAddr) {
	return e.Self.Find(name)
}

func (e ContainerElement) Address() (Address, int) {
	return e.Self.Address(), e.Index
}

func (e ContainerElement) Flatten() (*ContainerElement, []VisitAddr) {
	if e.Index >= len(e.Self.Contents) {
		if e.Self.ParentIndex == nil {
			return nil, nil
		}
		return e.Self.Parent.at(*e.Self.ParentIndex + 1), nil
	}
	container, ok := e.Node().(Container)
	addrs := []VisitAddr{{
		Addr:       e.Self.Address(),
		Flags:      e.Self.Flags,
		EntryIndex: e.Index,
	}}
	if !ok {
		return &e, addrs
	}
	container.Parent = &e.Self
	container.ParentIndex = ptr(e.Index)
	el, visits := ContainerElement{
		Self:  container,
		Index: 0,
	}.Flatten()
	return el, append(addrs, visits...)
}

func (e ContainerElement) Node() Node {
	return e.Self.Contents[e.Index]
}

func (e ContainerElement) Next() (Element, []VisitAddr) {
	next, visits := ContainerElement{
		Self:  e.Self,
		Index: e.Index + 1,
	}.Flatten()
	if next == nil {
		return nil, nil
	}
	return *next, visits
}

type Container struct {
	Name        string
	Parent      *Container
	ParentIndex *int
	Flags       ContainerFlag
	Contents    []Node
	Nested      map[string]Container
}

func (c Container) key() string {
	switch {
	case c.Name != "":
		return c.Name
	case c.ParentIndex != nil:
		return fmt.Sprint(*c.ParentIndex)
	default:
		return ""
	}
}

func (c Container) Address() Address {
	key := c.key()
	if c.Parent == nil || c.Parent.Parent == nil {
		return Address(key)
	}
	return c.Parent.Address() + Address("."+key)
}

type VisitAddr struct {
	Addr       Address
	Flags      ContainerFlag
	EntryIndex int
}

func (c *Container) Find(name Address) (Element, []VisitAddr) {
	n := string(name)
	if !strings.HasPrefix(n, ".^.") {
		return c.Root().Find(".^." + name)
	}
	path, key := splitAddress(n[1:])

	// This is a silly hack, but relative paths include a `.^.` at the start which
	// actually references the container including the current element, which is
	// already `c` here. To make this lookup more consistent, just add a parent so
	// that it comes back here for the right starting point.
	c = &Container{Parent: c}
	for i, p := range path {
		c = c.findContainer(p)
		if c == nil {
			panic(fmt.Errorf("container not found at %#v", path[:i+1]))
		}
	}
	return c.find(key)
}

func (c *Container) find(key string) (Element, []VisitAddr) {
	if index, err := strconv.Atoi(key); err == nil {
		return c.atNoFlatten(index).Flatten()
	}
	if child := c.findContainer(key); child != nil {
		return child.atNoFlatten(0).Flatten()
	}
	return nil, nil
}

func (c *Container) findContainer(key string) *Container {
	if key == "^" {
		return c.Parent
	}
	if index, err := strconv.Atoi(key); err == nil {
		child := c.Contents[index].(Container)
		child.Parent = c
		child.ParentIndex = ptr(index)
		return &child
	}
	if n, ok := c.Nested[key]; ok {
		child := n
		child.Parent = c
		return &child
	}
	for i, n := range c.Contents {
		if child, ok := n.(Container); ok && child.Name == key {
			child.Parent = c
			child.ParentIndex = ptr(i)
			return &child
		}
	}
	return nil
}

func (c *Container) Root() *Container {
	for ; c.Parent != nil; c = c.Parent {
	}
	return c
}

func (c Container) at(i int) *ContainerElement {
	elem, _ := c.atNoFlatten(i).Flatten()
	return elem
}

func (c Container) atNoFlatten(i int) ContainerElement {
	return ContainerElement{
		Self:  c,
		Index: i,
	}
}

func (c Container) First() Element {
	return c.at(0)
}

func ptr[T any](t T) *T {
	return &t
}

func splitAddress(s string) ([]string, string) {
	path := strings.Split(s, ".")
	return path[:len(path)-1], path[len(path)-1]
}
