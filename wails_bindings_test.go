package main

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The Wails bindings in frontend/wailsjs/go/main are generated, but they are
// routinely hand-edited when a method is added without a full `wails build`.
// Nothing else catches a mistake there: the frontend has no svelte-check and no
// TypeScript, so a missing or misspelled binding is a green build and a call
// that fails silently at runtime.
//
// This pins all three surfaces to each other by exact set equality, so it cannot
// pass vacuously.

var bindingExportRe = regexp.MustCompile(`(?m)^export function (\w+)\(([^)]*)\)`)

// countArgs counts top-level commas only. App.d.ts parameter types carry commas
// inside generics - Record<string, any> - so a naive count overstates the arity.
func countArgs(args string) int {
	if args == "" {
		return 0
	}
	n, depth := 1, 0
	for _, r := range args {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				n++
			}
		}
	}
	return n
}

func parseBindings(t *testing.T, path string) map[string]int {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]int{}
	for _, m := range bindingExportRe.FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = countArgs(strings.TrimSpace(m[2]))
	}
	if len(out) == 0 {
		t.Fatalf("parsed no exports from %s - the regex no longer matches the generated style", path)
	}
	return out
}

func appMethodArity(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	rt := reflect.TypeOf(&App{})
	for i := 0; i < rt.NumMethod(); i++ {
		m := rt.Method(i)
		// NumIn includes the receiver.
		out[m.Name] = m.Type.NumIn() - 1
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func diffNames(a, b map[string]int) []string {
	var only []string
	for k := range a {
		if _, ok := b[k]; !ok {
			only = append(only, k)
		}
	}
	sort.Strings(only)
	return only
}

func TestWailsBindingsMatchAppMethods(t *testing.T) {
	js := parseBindings(t, "frontend/wailsjs/go/main/App.js")
	dts := parseBindings(t, "frontend/wailsjs/go/main/App.d.ts")
	goMethods := appMethodArity(t)

	if only := diffNames(js, goMethods); len(only) > 0 {
		t.Errorf("App.js binds methods that do not exist on *App (these fail silently at runtime): %v", only)
	}
	if only := diffNames(goMethods, js); len(only) > 0 {
		t.Errorf("*App methods with no App.js binding (unreachable from the frontend): %v", only)
	}
	if only := diffNames(js, dts); len(only) > 0 {
		t.Errorf("in App.js but missing from App.d.ts: %v", only)
	}
	if only := diffNames(dts, js); len(only) > 0 {
		t.Errorf("in App.d.ts but missing from App.js: %v", only)
	}

	for _, name := range sortedKeys(js) {
		want, ok := goMethods[name]
		if !ok {
			continue // already reported above
		}
		if js[name] != want {
			t.Errorf("App.js %s takes %d args, *App.%s takes %d", name, js[name], name, want)
		}
		if dtsArity, ok := dts[name]; ok && dtsArity != want {
			t.Errorf("App.d.ts %s takes %d args, *App.%s takes %d", name, dtsArity, name, want)
		}
	}
}

// TestRenderBindingsAreWired names the two methods this package added by hand,
// so a regression points straight at them rather than at a set difference.
func TestRenderBindingsAreWired(t *testing.T) {
	js := parseBindings(t, "frontend/wailsjs/go/main/App.js")
	dts := parseBindings(t, "frontend/wailsjs/go/main/App.d.ts")
	goMethods := appMethodArity(t)

	for name, arity := range map[string]int{"HighlightSource": 2, "RenderTELAMarkdown": 1} {
		if goMethods[name] != arity {
			t.Errorf("*App.%s arity = %d, want %d", name, goMethods[name], arity)
		}
		if js[name] != arity {
			t.Errorf("App.js %s arity = %d, want %d", name, js[name], arity)
		}
		if dts[name] != arity {
			t.Errorf("App.d.ts %s arity = %d, want %d", name, dts[name], arity)
		}
	}
}
