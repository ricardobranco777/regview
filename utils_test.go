package main

import (
	"reflect"
	"regexp"
	"testing"

	"mvdan.cc/sh/v3/pattern"
)

func Test_filterRegex(t *testing.T) {
	ss := []string{"abcde", "abxde", "xyz"}

	xwant := map[string][]string{
		"ab?de":    {"abcde", "abxde"},
		"ab*":      {"abcde", "abxde"},
		"ab[cx]de": {"abcde", "abxde"},
		"*":        ss,
	}

	for pat, want := range xwant {
		expr, err := pattern.Regexp(pat, 0)
		if err != nil {
			panic(err)
		}
		regex := regexp.MustCompile("^" + expr + "$")

		got := filterRegex(ss, regex, false)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("filterRegex(%s, %s) got %v; want %v", ss, regex, got, want)
		}
	}
}

func Test_getMax(t *testing.T) {
	ss := []string{"", "abc", "abcdef", "abcde"}

	want := len("abcdef")
	got := getMax(ss)
	if got != want {
		t.Errorf("filterRegex(%v) got %d; want %d", ss, got, want)
	}
}
