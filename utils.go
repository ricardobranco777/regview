package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/docker/go-units"
	"golang.org/x/term"
)

func getMax(ss []string) (max int) {
	for _, s := range ss {
		if len(s) > max {
			max = len(s)
		}
	}
	return
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

func prettyTime(str string) string {
	tz, _ := time.LoadLocation("Local")
	t, _ := time.Parse(time.RFC3339Nano, str)
	return t.In(tz).Format(time.UnixDate)
}

func prettySize(n int64) string {
	return units.HumanSize(float64(n))
}

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
