package kline

import (
	"errors"
	"testing"
)

type kindLie struct{ *MA }

func (kindLie) Name() string     { return "MISKIND" }
func (kindLie) PaneKind() string { return PaneMain }

func TestRegisterRejectsKindMismatch(t *testing.T) {
	t.Cleanup(func() {
		next := registry[:0]
		for _, spec := range registry {
			if spec.Name != "MISKIND" && spec.Name != "NILIND" {
				next = append(next, spec)
			}
		}
		registry = next
	})
	err := Register(Spec{
		Name: "MISKIND",
		Kind: PaneSub,
		New:  func() Indicator { return kindLie{MA: NewMA(5, 20)} },
	})
	if !errors.Is(err, ErrIndicatorKind) {
		t.Fatalf("mismatch: %v", err)
	}
	for _, spec := range Registry() {
		if spec.Name == "MISKIND" {
			t.Fatal("mismatched indicator was registered")
		}
	}
	if err := Register(Spec{
		Name: "NILIND",
		Kind: PaneMain,
		New:  func() Indicator { return nil },
	}); err == nil {
		t.Fatal("nil constructor accepted")
	}
}
