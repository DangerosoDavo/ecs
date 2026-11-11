package ecs

import (
	"errors"
	"testing"
)

type cloneNode struct {
	Next   *cloneNode
	Values []int
}
type privateSnapshot struct{ value int }

type failingSnapshotClone struct{}

var snapshotCloneFailure = errors.New("cannot detach handle")

func (failingSnapshotClone) CloneForECS() (any, error) { return nil, snapshotCloneFailure }

type wrongSnapshotClone struct{}

func (wrongSnapshotClone) CloneForECS() (any, error) { return "wrong type", nil }

func (v privateSnapshot) CloneForECS() (any, error) { return privateSnapshot{value: v.value}, nil }
func TestSnapshotCyclesSlicesAndCustomCloner(t *testing.T) {
	n := &cloneNode{Values: []int{1}}
	n.Next = n
	copy, err := cloneSnapshot(n)
	if err != nil {
		t.Fatal(err)
	}
	cloned := copy.(*cloneNode)
	if cloned == n || cloned.Next != cloned {
		t.Fatal("pointer cycle not detached/preserved")
	}
	cloned.Values[0] = 2
	if n.Values[0] != 1 {
		t.Fatal("slice alias")
	}
	v, err := cloneSnapshot(privateSnapshot{value: 3})
	if err != nil || v.(privateSnapshot).value != 3 {
		t.Fatalf("custom cloner: %v %v", v, err)
	}
	_, err = cloneSnapshot(struct{ hidden int }{hidden: 1})
	if !errors.Is(err, ErrUnsupportedSnapshot) {
		t.Fatal("private mutable graph silently accepted")
	}
	slice := []int{1, 2}
	values := [][]int{slice[:1], slice}
	value, err := cloneSnapshot(values)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.([][]int)[0]) != 1 || len(value.([][]int)[1]) != 2 {
		t.Fatal("same-start slices with different lengths collapsed")
	}
}

func TestSnapshotHeterogeneousGraphAndNilValues(t *testing.T) {
	type data struct {
		ID         EntityID
		Array      [2][]int
		Dynamic    any
		NilPointer *int
		NilSlice   []int
		NilMap     map[string]int
	}
	original := data{ID: EntityIDFromParts(3, 2), Array: [2][]int{{1}, {2}}, Dynamic: map[string]any{"nested": []int{3}}}
	value, err := cloneSnapshot(original)
	if err != nil {
		t.Fatal(err)
	}
	copy := value.(data)
	if copy.ID != original.ID || copy.NilPointer != nil || copy.NilSlice != nil || copy.NilMap != nil {
		t.Fatal("identity or nil values changed")
	}
	copy.Array[0][0] = 9
	copy.Dynamic.(map[string]any)["nested"].([]int)[0] = 8
	if original.Array[0][0] != 1 || original.Dynamic.(map[string]any)["nested"].([]int)[0] != 3 {
		t.Fatal("array/interface graph aliased")
	}
	value, err = cloneSnapshot(nil)
	if err != nil || value != nil {
		t.Fatalf("nil graph changed: %v %v", value, err)
	}
	var empty any
	value, err = cloneSnapshot(struct{ Value any }{Value: empty})
	if err != nil || value.(struct{ Value any }).Value != nil {
		t.Fatal("nil interface changed")
	}
}

func TestSnapshotRejectsUnsupportedNestedGraphsAndBadCloners(t *testing.T) {
	for name, value := range map[string]any{
		"slice":            []chan int{make(chan int)},
		"array":            [1]chan int{make(chan int)},
		"map_value":        map[string]any{"handle": make(chan int)},
		"map_key":          map[chan int]int{make(chan int): 1},
		"interface":        struct{ Value any }{Value: func() {}},
		"pointer":          &struct{ Handle chan int }{Handle: make(chan int)},
		"wrong_clone_type": wrongSnapshotClone{},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := cloneSnapshot(value); !errors.Is(err, ErrUnsupportedSnapshot) {
				t.Fatalf("unsupported handle escaped: %v", err)
			}
		})
	}
	if _, err := cloneSnapshot(failingSnapshotClone{}); !errors.Is(err, snapshotCloneFailure) {
		t.Fatalf("custom failure hidden: %v", err)
	}
}
