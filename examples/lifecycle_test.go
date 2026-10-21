// Package examples_test pins the window lifecycle every runnable example
// follows: the window is created from WhenReady, after the native app is
// initialized, and the app event loop starts only after that callback is
// registered. Creating a window before Run exits with an error.
package examples_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleWindowsAreCreatedWhenReady(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "main.go")
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(src)
		if !strings.Contains(text, "mygo.NewWindow") {
			continue
		}
		checked++
		ready := strings.Index(text, "mygo.App.WhenReady(")
		window := strings.Index(text, "mygo.NewWindow(")
		run := strings.Index(text, "mygo.App.Run()")
		if ready < 0 {
			t.Errorf("%s: mygo.NewWindow is called outside mygo.App.WhenReady", path)
			continue
		}
		if window < ready || (run >= 0 && window > run) {
			t.Errorf("%s: NewWindow must be inside WhenReady and before App.Run", path)
		}
	}
	if checked < 6 {
		t.Fatalf("only %d window examples were checked", checked)
	}
	t.Logf("checked %d window examples", checked)
}

func TestHeadlessFlagsRunBeforeWhenReady(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"extend", "replay"} {
		src, err := os.ReadFile(filepath.Join(root, name, "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		headless := strings.Index(text, "if *headless {")
		ready := strings.Index(text, "mygo.App.WhenReady(")
		if headless < 0 || ready < 0 || headless > ready {
			t.Errorf("%s: -headless must return before mygo.App.WhenReady", name)
		}
	}
}
