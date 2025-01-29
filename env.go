// A library to provide a bash-compatible "expand environment
// variable" function.
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// An interface to allow us to more easily test this, as some of the
// expansions are state-modifying. This way, we can ensure that we
// have a known state for testing.
type Environment interface {
	Set(name, value string)
	Get(name string) (string, bool)
}

type native struct{}

// Set variables in the proper environment
func (e native) Set(name, value string) {
	os.Setenv(name, value)
}

// Look up variables in the proper environment
func (e native) Get(name string) (string, bool) {
	return os.LookupEnv(name)
}

type internal map[string]string

func (i internal) Set(name, value string) {
	i[name] = value
}

func (i internal) Get(name string) (string, bool) {
	v, ok := i[name]
	return v, ok
}

// Find the next "looks like the start of a variable expansion" in a
// string, starting at a given offset. Return -1 to indicate "no next".
func findNextStart(s string, p int) int {
	end := len(s)
	state := 0

	for p < end {
		switch state {
		case 0: // Base state
			switch s[p] {
			case '$':
				return p
			case '\\':
				// next is escaped
				state = 1
			}
		case 1: // Backslash-escaped state
			state = 0
		}
		p++
	}
	return -1
}

// Return true if this looks like a character in a shell variable name.
func nameConstituent(c byte) bool {
	switch {
	case (c >= 'A') && (c <= 'Z'):
		return true
	case (c >= 'a') && (c <= 'z'):
		return true
	case (c >= '0') && (c <= '9'):
		return true

	}

	return false
}

// return true if this looks like a constituent in a positional parameter
func parameterConstituent(c byte) bool {
	switch {
	case (c >= '0') && (c <= '9'):
		return true
	}
	return false
}

// Return the next position in s that any of the characters in charset is in.
// A bit naughtily, overload "not found" to be -1
func skipToNext(s, charset string, i int) int {
	nexts := make(map[rune]bool)
	for _, c := range charset {
		nexts[c] = true
	}

	end := len(s)
	for i < end {
		if nexts[rune(s[i])] {
			return i
		}
		i++
	}

	return -1
}

// Find the end of a variable expansion, given a start position
func findNextEnd(s string, p int) int {
	end := len(s)
	// We expect to be in one of a few states:
	//  0: looking at the start of the variable
	//  1: found a {
	//  2: no { at the start, not a digit
	//  3: a digit
	state := 0

	for p < end {
		switch state {
		case 0:
			switch s[p] {
			case '$':
				state = 0
			case '{':
				state = 1
			case '0':
				state = 3
			case '1':
				state = 3
			case '2':
				state = 3
			case '3':
				state = 3
			case '4':
				state = 3
			case '5':
				state = 3
			case '6':
				state = 3
			case '7':
				state = 3
			case '8':
				state = 3
			case '9':
				state = 3
			default:
				state = 2
			}
		case 1:
			if s[p] == '}' {
				return p + 1
			}
		case 2:
			c := s[p]
			if !nameConstituent(c) {
				return p
			}
		case 3:
			return p
		}
		p++
	}

	return end
}

// Gneeral interface for expansions. This simply has a single `expand`
// method that returns the expansion given a specific environment.
type expansion interface {
	expand(Environment) string
}

type positional int

// Positional parameters, note that these do NOT use the envionment,
// but we pass it in to fulfill the interface contract.
func (p positional) expand(e Environment) string {
	if int(p) >= len(os.Args) {
		return ""
	}
	return os.Args[int(p)]
}

type constant string

// These are constant strings, we simply need to provide a method for
// them to be stored as expansions.
func (c constant) expand(e Environment) string {
	return string(c)
}

type normal string

// A normal variable expansion, like "$foo" or "${foo}".
func (n normal) expand(e Environment) string {
	v, _ := e.Get(string(n))

	return v
}

type indirect struct {
	name string
}

// Indirect expansion, "${!foo}", this first expands foo, then uses
// that for a second "normal" expansion.
func (i indirect) expand(e Environment) string {
	next, ok := e.Get(i.name)
	if !ok {
		return ""
	}

	v, _ := e.Get(next)
	return v
}

type defaulted struct {
	name  string
	word  expansion
	unset bool
}

// Defaulted expansion, like "${foo:-default}" or "$foo-default}"
func (d defaulted) expand(e Environment) string {
	v, ok := e.Get(d.name)

	if !ok {
		return d.word.expand(e)
	}

	if !d.unset {
		if v == "" {
			return d.word.expand(e)
		}
	}

	return v
}

func makeDefaulted(s string, i int, name string, unsetOnly bool) (expansion, error) {
	var err error

	rv := defaulted{name: name, unset: unsetOnly}

	if s[i] == '$' {
		rv.word, err = parseExpansion(s, i)
		if err != nil {
			return rv, err
		}
	} else {
		end := i
		for s[end] != '}' {
			end++
		}
		rv.word = constant(s[i:end])
	}

	return rv, nil
}

type assign struct {
	name  string
	word  expansion
	unset bool
}

// Assignment expansion, like "${foo:=default}" or "${foo=default}"
func (a assign) expand(e Environment) string {
	v, ok := e.Get(a.name)
	if !ok {
		v = a.word.expand(e)
		e.Set(a.name, v)
	}

	return v
}

func makeAssigned(s string, i int, name string, unsetOnly bool) (expansion, error) {
	var err error
	end := findNextEnd(s, i)
	rv := assign{name: name, unset: unsetOnly}
	if s[i] == '$' {
		end = findNextEnd(s, end+1)
		rv.word, err = parseExpansion(s, i)
		if err != nil {
			return rv, err
		}
	} else {
		rv.word = constant(s[i:end])
	}

	return rv, nil

}

type alternate struct {
	name  string
	word  expansion
	unset bool
}

// Alternate expansion, this is "${foo:+alternate}" or
// "${foo+alternate}", the alternate is substituted if foo has a
// value, and is otherwise blank.
func (a alternate) expand(e Environment) string {
	v, ok := e.Get(a.name)

	if !ok {
		return ""
	}

	if v == "" && !a.unset {
		return ""
	}

	return a.word.expand(e)
}

func makeAlternated(s string, i int, name string, unsetOnly bool) (expansion, error) {
	var err error

	rv := alternate{name: name, unset: unsetOnly}
	end := findNextEnd(s, i)

	if s[i] == '$' {
		end = findNextEnd(s, end+1)
		rv.word, err = parseExpansion(s, i)
		if err != nil {
			return rv, err
		}
	} else {
		rv.word = constant(s[i:end])
	}

	return rv, nil
}

type offset struct {
	name   string
	offset int
	length int
	useLen bool
}

// Offset expansion, this is "${foo:<offset>}" or
// "${foo:<offset>:<length>}". There's some complicated "what happens
// if there are negative numbers" behaviour, please cross-reference
// the bash manual for specifics.
func (o offset) expand(e Environment) string {
	v, ok := e.Get(o.name)
	if !ok {
		return ""
	}

	l := len(v)

	b := o.offset
	if b < 0 {
		b = l + b
	}
	if b < 0 {
		return ""
	}

	end := l
	if o.useLen {
		if o.length > 0 {
			end = b + o.length
		} else {
			end = l + o.length
		}
	}
	if end > l {
		end = l
	}
	if end < b {
		return ""
	}

	return v[b:end]
}

func makeOffseted(s string, i int, name string) (expansion, error) {
	rv := offset{name: name}

	for s[i] == ':' {
		i++
	}
	// Skip possible spaces
	for s[i] == ' ' {
		i++
	}

	// Parse first offset
	next := skipToNext(s, ":}", i)
	n, err := strconv.Atoi(s[i:next])
	if err != nil {
		return rv, err
	}
	rv.offset = n

	if s[next] == ':' {
		// We have a length
		i = next + 1
		// Skip possible spaces
		for s[i] == ' ' {
			i++
		}
		end := skipToNext(s, "}", i)
		n, err := strconv.Atoi(s[i:end])
		if err != nil {
			return rv, err
		}
		rv.useLen = true
		rv.length = n
	}

	return rv, nil
}

type length struct {
	name string
}

func (l length) expand(e Environment) string {
	value, _ := e.Get(l.name)
	return fmt.Sprintf("%d", len(value))
}

type match struct {
	name    string
	pattern string
	longest bool
	suffix  bool
}

// Expand matches, we are using the same type for lonmgest/shortest
// prefix/suffix match, as it's pretty much the same logic throughout.
func (m match) expand(e Environment) string {
	v, ok := e.Get(m.name)
	if !ok {
		return ""
	}

	l := len(v)

	if m.suffix {
		if m.longest {
			for o := 0; o < l; o++ {
				matched, err := filepath.Match(m.pattern, v[o:])
				if err != nil {
					return ""
				}
				if matched {
					return v[:o]
				}
			}
		} else {
			for o := l; o >= 0; o-- {
				matched, err := filepath.Match(m.pattern, v[o:])
				if err != nil {
					return ""
				}
				if matched {
					return v[:o]
				}
			}
		}
	} else {
		if m.longest {
			for o := l; o >= 0; o-- {
				matched, err := filepath.Match(m.pattern, v[:o])
				if err != nil {
					return ""
				}
				if matched {
					return v[o:]
				}
			}
		} else {
			for o := 0; o < l; o++ {
				matched, err := filepath.Match(m.pattern, v[:o])
				if err != nil {
					return ""
				}
				if matched {
					return v[o:]
				}
			}
		}
	}

	return v
}

// Tweak a POSIX shell pattern and make it into something that works
// with filepath.Match. For some reason, the system library has chosen
// [^...] as a character negation class, instead of [!...]. Luckily
// bash supports both, so by changing ! to ^, we're doing the right
// thing. There may be edge cases where we get this wrong...
func manglePattern(pattern string) string {
	var acc strings.Builder
	state := 0

	acc.Grow(len(pattern))

	for _, r := range pattern {
		switch state {
		case 0:
			if r == rune('[') {
				state = 1
			}
		case 1:
			if r == rune('!') {
				r = rune('^')
			}
			state = 0
		}
		acc.WriteRune(r)
	}

	return acc.String()
}

func makeMatch(s string, i int, name string, suffix bool) expansion {
	check := map[bool]byte{false: '#', true: '%'}
	rv := match{name: name, suffix: suffix}
	if s[i] == check[suffix] {
		i++
		rv.longest = true
	}
	end := skipToNext(s, "}", i)
	rv.pattern = manglePattern(s[i:end])

	return rv
}

// Parse the correct type of expansion from a string at a given
// offset, we expect the caller to already know where it ends, for
// purposes of string slicing.
func parseExpansion(s string, o int) (expansion, error) {
	if s[o] != '$' {
		return constant("failed"), fmt.Errorf("Unexpected first character, %c", s[o])
	}

	switch {
	case parameterConstituent(s[o+1]):
		p, err := strconv.Atoi(s[o+1 : o+2])
		if err != nil {
			return constant("positional parse failed"), err
		}
		return positional(p), nil
	case nameConstituent(s[o+1]):
		p := o + 1
		l := len(s)
		for p < l && nameConstituent(s[p]) {
			p++
		}
		return normal(s[o+1 : p]), nil
	case s[o+1] == '{':
		for i := o + 2; i < len(s); i++ {
			c := s[i]
			switch c {
			case '!':
				if i == o+2 {
					// We are looking at an indirect expansion
					end := findNextEnd(s, i)
					rv := indirect{name: s[i+1 : end]}
					return rv, nil
				}
			case '#':
				if i == o+2 {
					// We are looking at a length expansion
					end := findNextEnd(s, i)
					rv := length{name: s[i+1 : end]}
					return rv, nil
				}
				// Prefix match
				return makeMatch(s, i+1, s[o+2:i], false), nil
			case '%':
				// Suffix match
				return makeMatch(s, i+1, s[o+2:i], true), nil
			case ':':
				switch {
				case s[i+1] == '-':
					return makeDefaulted(s, i+2, s[o+2:i], false)
				case s[i+1] == '=':
					return makeAssigned(s, i+2, s[o+2:i], false)
				case s[i+1] == '+':
					return makeAlternated(s, i+2, s[o+2:i], false)
				}
				return makeOffseted(s, i, s[o+2:i])
			case '-':
				return makeDefaulted(s, i+1, s[o+2:i], true)
			case '=':
				return makeAssigned(s, i+1, s[o+2:i], true)
			case '+':
				return makeAlternated(s, i+1, s[o+2:i], true)
			case '}':
				return normal(s[o+2 : i]), nil
			}

		}
	}
	return constant("failed"), fmt.Errorf("Expected to have been caught by a switch statement")
}

// Expand a string, with a given environment. Return the expanded
// string and the first error encountered while expanding the string.
func expand(s string, e Environment) (string, error) {
	var parts []string
	offset := 0
	done := false

	for !done {
		next := findNextStart(s, offset)
		if next == -1 {
			parts = append(parts, s[offset:])
			done = true
			continue
		}

		parts = append(parts, s[offset:next])
		exp, err := parseExpansion(s, next)
		if err != nil {
			return "An error occurred", err
		}
		offset = findNextEnd(s, next)
		parts = append(parts, exp.expand(e))
	}

	return strings.Join(parts, ""), nil
}

// Expand a string using the os Environment. Return the expanded
// string and/or errors encountered during the parsing.
func Expand(s string) (string, error) {
	return expand(s, native{})
}

// Expand a string using a passed-in environment. Return the expanded
// string and/or erors encountered during the parsing.
func ExpandWithEnvironment(s string, e Environment) (string, error) {
	return expand(s, e)
}
