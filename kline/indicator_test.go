package kline

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

type kindLie struct{ *MA }

func (kindLie) Name() string     { return "MISKIND" }
func (kindLie) PaneKind() string { return PaneMain }

func TestRegisterRejectsKindMismatch(t *testing.T) {
	t.Cleanup(func() {
		Unregister("MISKIND")
		Unregister("NILIND")
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

func TestRegistryReadRacesRegister(t *testing.T) {
	const n = 80
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("RACE%04d", i)
	}
	t.Cleanup(func() {
		for _, name := range names {
			Unregister(name)
		}
	})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, name := range names {
			_ = Register(Spec{
				Name: name,
				Kind: PaneMain,
				New:  func() Indicator { return NewMA(5, 20) },
			})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			_ = Registry()
		}
	}()
	wg.Wait()
}
