package main

import (
	"flag"
	"strings"
)

// flagSet is a thin wrapper over flag.FlagSet that keeps the argument slice so
// each subcommand can define flags then read any positional arguments.
type flagSet struct {
	fs   *flag.FlagSet
	args []string
}

func newFlagSet(args []string) *flagSet {
	return &flagSet{fs: flag.NewFlagSet("mytraffic", flag.ContinueOnError), args: args}
}

func (f *flagSet) String(name, def, usage string) *string { return f.fs.String(name, def, usage) }
func (f *flagSet) Bool(name string, def bool, usage string) *bool { return f.fs.Bool(name, def, usage) }
func (f *flagSet) Int(name string, def int, usage string) *int { return f.fs.Int(name, def, usage) }
func (f *flagSet) parse()                                     { _ = f.fs.Parse(f.args) }
func (f *flagSet) rest() []string                            { return f.fs.Args() }

// extractPositional returns the first all-digit argument (the flow id), plus the
// remaining arguments with it removed. Restricting to digits means it is never
// confused with a flag or a flag's value such as an --api URL, so the id may
// appear before or after flags.
func extractPositional(args []string) (pos string, rest []string) {
	rest = make([]string, 0, len(args))
	taken := false
	for _, a := range args {
		if !taken && isDigits(a) {
			pos = a
			taken = true
			continue
		}
		rest = append(rest, a)
	}
	return pos, rest
}

// valueFlags are the flags that consume the following argument, so extractPath
// can skip a flag's value when hunting for the positional path.
var valueFlags = map[string]bool{"--api": true, "-api": true}

// extractPath returns the first positional (non-flag) argument — a file path —
// plus the remaining arguments, so the path may appear before or after flags.
func extractPath(args []string) (path string, rest []string) {
	rest = make([]string, 0, len(args))
	taken := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			// If this flag takes a value and uses the separate-arg form, keep its value with it.
			if valueFlags[a] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				rest = append(rest, args[i])
			}
			continue
		}
		if !taken {
			path = a
			taken = true
			continue
		}
		rest = append(rest, a)
	}
	return path, rest
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
