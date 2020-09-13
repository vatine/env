// A package to provide a bash-compatible "expand environment
// variable" function
package env

import (
	"fmt"
	"os"
	"strconv"
)

type environment interface {
	set(name, value string)
	get(name string) (string, bool)
}

type native struct{}

func (e native) set(name, value string) {
	os.Setenv(name, value)
}

func (e native) get(name string) (string, bool) {
	return os.LookupEnv(name)
}

type internal map[string]string

func (i internal) set(name, value string) {
	i[name] = value
}

func (i internal) get(name string) (string, bool) {
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

func parameterConstituent(c byte) bool {
	switch {
	case (c >= '0') && (c <= '9'):
		return true
	}
	return false
}

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

type expansion interface {
	expand(environment) string
}

type positional int

func (p positional) expand(e environment) string {
	if int(p) >= len(os.Args) {
		return ""
	}
	return os.Args[int(p)]
}

type constant string

func (c constant) expand(e environment) string {
	return string(c)
}

type normal string

func (n normal) expand(e environment) string {
	v, _ := e.get(string(n))

	return v
}

type indirect struct {
	name string
}

func (i indirect) expand(e environment) string {
	next, ok := e.get(i.name)
	if !ok {
		return ""
	}

	v, _ := e.get(next)
	return v
}

type defaulted struct {
	name  string
	word  expansion
	unset bool
}

func (d defaulted) expand(e environment) string {
	v, ok := e.get(d.name)

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

func (a assign) expand(e environment) string {
	v, ok := e.get(a.name)
	if !ok {
		v = a.word.expand(e)
		e.set(a.name, v)
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

func (a alternate) expand(e environment) string {
	v, ok := e.get(a.name)

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

func (o offset) expand(e environment) string {
	v, ok := e.get(o.name)
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

// Parse the correct type of expansion from a string at a given
// offset, we expect the caller to already know where it ends, for
// purposes of string slicing. This is basically just the entry-point.
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
