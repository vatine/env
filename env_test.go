package env

import (
	"testing"
)

func TestFindStart0(t *testing.T) {
	cases := []struct {
		s string
		e int
	}{
		{"apap$foo", 4}, {"apapap", -1},
	}

	for ix, c := range cases {
		expected := c.e
		seen := findNextStart(c.s, 0)
		if seen != expected {
			t.Errorf("Case %d, saw %d expected %d", ix, seen, expected)
		}
	}
}

func TestFindStart1(t *testing.T) {
	cases := []struct {
		s string
		o int
		e int
	}{
		{"apap$foo", 0, 4}, {"apap$foo", 5, -1}, {"ap$ap$foo", 0, 2},
		{"ap$ap$foo", 1, 2}, {"ap$ap$foo", 2, 2}, {"ap$ap$foo", 3, 5},
		{"ap$ap$foo", 6, -1},
	}
	for ix, c := range cases {
		expected := c.e
		seen := findNextStart(c.s, c.o)
		if seen != expected {
			t.Errorf("Case %d, saw %d expected %d", ix, seen, expected)
		}
	}
}

func TestFindEnd(t *testing.T) {
	cases := []struct {
		s string
		e int
	}{
		{"$11", 2}, {"$apa ", 4}, {"${11}", 5}, {"${foo}", 6},
	}
	for ix, c := range cases {
		expected := c.e
		seen := findNextEnd(c.s, 0)
		if seen != expected {
			t.Errorf("Case %d, saw %d expected %d", ix, seen, expected)
		}
	}
}

func TestExpand(t *testing.T) {
	e := internal{"foo": "bar", "bar": "gazonk", "empty": ""}
	cases := []struct {
		d expansion
		e string
	}{
		{indirect{"foo"}, "gazonk"},
		{indirect{"bar"}, ""},
		{normal("foo"), "bar"},
		{normal("bar"), "gazonk"},
		{defaulted{name: "foo", word: constant("blubb")}, "bar"},
		{defaulted{"bar", constant("blubb"), false}, "gazonk"},
		{defaulted{"slem", constant("blubb"), false}, "blubb"},
		{defaulted{"slem", normal("bar"), false}, "gazonk"},
		{defaulted{"bar", normal("foo"), false}, "gazonk"},
		{defaulted{"empty", normal("foo"), false}, "bar"},
		{defaulted{"empty", normal("foo"), true}, ""},
		{offset{"bar", 0, 0, false}, "gazonk"},
		{offset{"bar", 0, 2, false}, "gazonk"},
		{offset{"bar", 0, -1, true}, "gazon"},
		{offset{"bar", 0, -1, false}, "gazonk"},
		{offset{"bar", -6, 11, true}, "gazonk"},
		{offset{"bar", 0, 3, true}, "gaz"},
		{offset{"bar", 2, 3, false}, "zonk"},
		{offset{"bar", 2, 3, true}, "zon"},
		{offset{"bar", 2, -3, true}, "z"},
		{offset{"bar", 2, 4711, true}, "zonk"},
		{alternate{"unset", constant("text"), false}, ""},
		{alternate{"empty", constant("text"), true}, "text"},
		{alternate{"empty", constant("text"), false}, ""},
		{alternate{"foo", constant("text"), false}, "text"},
		{alternate{"foo", constant("text"), true}, "text"},
	}

	for ix, c := range cases {
		expected := c.e
		seen := c.d.expand(e)
		if seen != expected {
			t.Errorf("%d, saw «%s», expected «%s»", ix, seen, expected)
		}

	}
}

func TestParseExpansion1(t *testing.T) {
	e := internal{"foo": "bar", "bar": "gazonk", "empty": ""}

	// Note, these cases are run in order, and some have side effects
	cases := []struct {
		in   string
		want string
	}{
		{"$foo", "bar"},
		{"${foo:=foo}", "bar"},
		{"${foo}", "bar"},
		{"${foo:-unseen}", "bar"},
		{"${food:-unseen}", "unseen"},
		{"${empty:-unseen}", "unseen"},
		{"${empty-unseen}", ""},
		{"${new}", ""},
		{"${new:=foo}", "foo"},
		{"${!new}", "bar"},
		{"${unset:+text}", ""},
		{"${unset+text}", ""},
		{"${empty:+text}", ""},
		{"${empty+text}", "text"},
		{"${foo:+text}", "text"},
		{"${bar:2}", "zonk"},
		{"${bar:2:3}", "zon"},
		{"${bar:2:-3}", "z"},
		{"${bar:-2:-3}", "gazonk"},
		{"${unset:-2:-3}", "2:-3"},
		{"${#foo}", "3"},
		{"${#bar}", "6"},
		{"${#empty}", "0"},
	}

	for ix, c := range cases {
		parsed, _ := parseExpansion(c.in, 0)
		seen := parsed.expand(e)
		if seen != c.want {
			t.Errorf("Case %d, (%s) saw «%s», wanted «%s»", ix, c.in, seen, c.want)
		}
	}
}

func TestMainExpand(t *testing.T) {
	e := internal{"foo": "bar", "bar": "gazonk", "empty": ""}

	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{"a${foo}b", "abarb", false},
	}

	for ix, c := range cases {
		seen, err := expand(c.in, e)
		if err != nil && !c.err {
			t.Errorf("Case %d, unexpected error, %s", ix, err)
		}
		if err == nil && c.err {
			t.Errorf("Case %d, unexpected lack of error", ix)
		}
		if seen != c.want {
			t.Errorf("Case %d, (%s) saw «%s», wanted «%s»", ix, c.in, seen, c.want)
		}
	}
}
