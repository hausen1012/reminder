package notifier

import "testing"

func TestRender_BasicSubstitution(t *testing.T) {
	got := Render("你好 {{name}}，今天 {{date}}", map[string]string{
		"name": "张三",
		"date": "2026-06-05",
	})
	want := "你好 张三，今天 2026-06-05"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_UnknownPlaceholderPreserved(t *testing.T) {
	got := Render("{{title}} - {{missing}}", map[string]string{"title": "提醒"})
	want := "提醒 - {{missing}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_NoRecursion(t *testing.T) {
	// vars 里的值即使再含占位符也不会二次展开
	got := Render("{{x}}", map[string]string{"x": "{{y}}", "y": "should-not-appear"})
	want := "{{y}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_WhitespaceAround(t *testing.T) {
	got := Render("{{  name  }} 你好", map[string]string{"name": "李四"})
	want := "李四 你好"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_DotInName(t *testing.T) {
	got := Render("{{user.name}}", map[string]string{"user.name": "王五"})
	if got != "王五" {
		t.Errorf("got %q, want %q", got, "王五")
	}
}

func TestRender_EmptyTemplate(t *testing.T) {
	if got := Render("", map[string]string{"a": "b"}); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestRender_NilVars(t *testing.T) {
	got := Render("{{x}} y", nil)
	if got != "{{x}} y" {
		t.Errorf("got %q, want %q", got, "{{x}} y")
	}
}

func TestRenderMap(t *testing.T) {
	in := map[string]string{
		"key1": "{{a}}",
		"key2": "static",
	}
	out := RenderMap(in, map[string]string{"a": "value"})
	if out["key1"] != "value" || out["key2"] != "static" {
		t.Errorf("got %+v", out)
	}
}

func TestHasPlaceholder(t *testing.T) {
	if !HasPlaceholder("{{x}}") {
		t.Error("expect HasPlaceholder true for {{x}}")
	}
	if HasPlaceholder("no placeholder") {
		t.Error("expect HasPlaceholder false for plain text")
	}
	if HasPlaceholder("{{ }}") {
		t.Error("expect HasPlaceholder false for malformed placeholder")
	}
}
