package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/docker/go-units"
	"golang.org/x/term"
)

var tz *time.Location

func filterRegex(ss []string, regex *regexp.Regexp) []string {
	if regex == nil {
		return ss
	}

	var matched []string
	for _, s := range ss {
		if regex.MatchString(s) {
			matched = append(matched, s)
		}
	}

	return matched
}

func getCommit() string {
	var commit, dirty string

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch {
			case setting.Key == "vcs.revision":
				commit = setting.Value
			case setting.Key == "vcs.modified":
				dirty = "-dirty"
			}
		}
	}

	return commit + dirty
}

func getMax(ss []string) int {
	max := 0

	for _, s := range ss {
		if len(s) > max {
			max = len(s)
		}
	}

	return max
}

func getPass(prompt string, args ...interface{}) string {
	fmt.Fprintf(os.Stderr, prompt, args...)

	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintln(os.Stderr)

	return string(pass)
}

func prettySize(n int64) string {
	return units.HumanSize(float64(n))
}

func prettyTime(t *time.Time) string {
	return t.In(tz).Format(time.UnixDate)
}
