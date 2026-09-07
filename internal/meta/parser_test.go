package meta

import "testing"

func TestParseBasic(t *testing.T) {
	c := "Title=My Comic\nAuthor=Someone\n\nChapter=10 = special\n"
	m := Parse(c)
	if m["Title"] != "My Comic" {
		t.Errorf("Title = %q", m["Title"])
	}
	if m["Author"] != "Someone" {
		t.Errorf("Author = %q", m["Author"])
	}
	if m["Chapter"] != "10 = special" {
		t.Errorf("value containing '=' = %q", m["Chapter"])
	}
}

func TestParseSkipsJunk(t *testing.T) {
	m := Parse("this line has no equals\n = empty key\nKey=value\r\n\r\n")
	if len(m) != 1 {
		t.Errorf("len = %d, want 1 (only Key)", len(m))
	}
	if m["Key"] != "value" {
		t.Errorf("Key = %q", m["Key"])
	}
}

func TestParseEmpty(t *testing.T) {
	if m := Parse(""); len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
	if m := Parse("   \n  \n"); len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestParseUTF8(t *testing.T) {
	m := Parse("标题=测试漫画\n作者=张三")
	if m["标题"] != "测试漫画" {
		t.Errorf("标题 = %q", m["标题"])
	}
	if m["作者"] != "张三" {
		t.Errorf("作者 = %q", m["作者"])
	}
}
