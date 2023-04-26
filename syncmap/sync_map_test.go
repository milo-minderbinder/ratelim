package syncmap

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	m := New[string, int]()
	fmt.Printf("%v, %T\n", m, m)
	if m == nil {
		t.Fatalf("instantiation failed: %#v, %T\n", m, m)
	}
	if m.wrapped == nil {
		t.Fatal("underlying map not initialized:", m)
	}
}
