package parsec

import (
	"container/vector";
	"reflect";
	"strings";
	"unicode";
)


// Container of the input, position, and any user/parser state.
type Vessel interface {
	GetState() State;
	SetState(State);

	GetInput() Input;
	SetInput(Input);

	GetPosition() Position;
	SetPosition(Position);

	GetSpec() Spec;
	SetSpec(Spec);

	Get(int) (Input, bool);
	Next() (int, bool);
	Pop(int);
	Push(int);
}

// Specifications for the parser
type Spec struct {
	CommentStart	string;
	CommentEnd	string;
	CommentLine	string;
	NestedComments	bool;
	IdentStart	Parser;
	IdentLetter	Parser;
	OpStart		Parser;
	OpLetter	Parser;
	ReservedNames	[]string;
	ReservedOpNames	[]string;
	CaseSensitive	bool;
}

// A Parser is a function that takes a vessel and returns any matches
// (Output) and whether or not the match was valid.
type Parser func(Vessel) (Output, bool)

// Input type used by vessels
type Input interface{}

// Output of Parsers
type Output interface{}

// Any value can be a vessel's state.
type State interface{}

// Position in the input.
type Position struct {
	Name	string;
	Line	int;
	Column	int;
	Offset	int;
}


// Token that satisfies a condition.
func Satisfy(check func(c int) bool) Parser {
	return func(in Vessel) (Output, bool) {
		target, ok := in.Next();
		if ok && check(target) {
			in.Pop(1);
			return target, true;
		}

		return nil, false;
	}
}

// Skip whitespace (TODO: Comments)
func Whitespace(in Vessel) (Output, bool)	{ return Many(Satisfy(unicode.IsSpace))(in) }

// Match a parser and skip whitespace
func Lexeme(match Parser) Parser {
	return func(in Vessel) (Output, bool) {
		out, matched := match(in);
		Whitespace(in);
		return out, matched;
	}
}

// Match a parser 0 or more times.
func Many(match Parser) Parser {
	return func(in Vessel) (Output, bool) {
		matches := new(vector.Vector);
		for {
			out, parsed := match(in);
			if !parsed {
				break
			}

			matches.Push(out);
		}

		return matches.Data(), true;
	}
}

// Match a parser seperated by another parser 0 or more times.
// Trailing delimeters are valid.
func SepBy(delim Parser, match Parser) Parser {
    return func(in Vessel) (Output, bool) {
        matches := new(vector.Vector);
        for {
            out, parsed := match(in);
            if !parsed {
                break
            }

            matches.Push(out);

            _, sep := delim(in);
            if !sep {
                break
            }
        }

        return matches, true
    }
}

// Go through the parsers until one matches.
func Any(parsers ...) Parser {
	return func(in Vessel) (Output, bool) {
		p := reflect.NewValue(parsers).(*reflect.StructValue);

		for i := 0; i < p.NumField(); i++ {
			parser := p.Field(i).Interface().(Parser);
			match, ok := parser(in);
			if ok {
				return match, ok
			}
		}

		return nil, false;
	}
}

// Match all parsers, returning the final result. If one fails, it stops.
// NOTE: this will not revert the state upon failure. Wrap calls in Try(...).
func All(parsers ...) Parser {
	return func(in Vessel) (match Output, ok bool) {
		p := reflect.NewValue(parsers).(*reflect.StructValue);

		for i := 0; i < p.NumField(); i++ {
			parser := p.Field(i).Interface().(Parser);
			match, ok = parser(in);
			if !ok {
				return
			}
		}

		return;
	}
}

// Match all parsers, collecting their outputs into a vector.
// If one parser fails, the whole thing fails.
// NOTE: this will not revert the state upon failure. Wrap calls in Try(...).
func Collect(parsers ...) Parser {
	return func(in Vessel) (Output, bool) {
		p := reflect.NewValue(parsers).(*reflect.StructValue);

		matches := new(vector.Vector);
		for i := 0; i < p.NumField(); i++ {
			parser := p.Field(i).Interface().(Parser);
			match, ok := parser(in);
			if !ok {
				return nil, false
			}

			matches.Push(match);
		}

		return matches, true;
	}
}

// Try matching begin, match, and then end.
func Between(begin Parser, end Parser, match Parser) Parser {
	return func(in Vessel) (Output, bool) {
		parse, ok := Try(Collect(begin, match, end))(in);
		if !ok {
			return nil, false
		}

		return parse.(*vector.Vector).At(1), true;
	}
}

// Lexeme parser for `match' wrapped in parens.
func Parens(match Parser) Parser	{ return Lexeme(Between(Symbol("("), Symbol(")"), match)) }

// Match a string and consume any following whitespace.
func Symbol(str string) Parser	{ return Lexeme(String(str)) }

// Match a string and pop the string's length from the input.
func String(str string) Parser {
	return func(in Vessel) (Output, bool) {
		if strings.HasPrefix(in.GetInput().(string), str) {
			in.Pop(len(str));
			return str, true;
		}

		return nil, false;
	}
}

// Try a parse and revert the state and position if it fails.
func Try(match Parser) Parser {
	return func(in Vessel) (Output, bool) {
		st, pos := in.GetState(), in.GetPosition();
		out, ok := match(in);
		if !ok {
			in.SetState(st);
			in.SetPosition(pos);
		}

		return out, ok;
	}
}

func Identifier(in Vessel) (name Output, ok bool) {
	sp := in.GetSpec();
	n, ok := sp.IdentStart(in);
	if !ok {
		return
	}

	ns, ok := Many(sp.IdentLetter)(in);
	if !ok {
		return
	}

	rest := make([]int, len(ns.([]interface{})));
	for k, v := range ns.([]interface{}) {
		rest[k] = v.(int)
	}

	return string(n.(int)) + string(rest), ok;
}

// Helper for passing a parser by reference, e.g. for
// infinite recursion: as := Many(Any(a, Parens(R(as))))
func R(parser *Parser) Parser {
	return func(in Vessel) (Output, bool) { return (*parser)(in) }
}

// Basic string vessel for parsing over a string input.
type StringVessel struct {
	state		State;
	input		string;
	position	Position;
	spec		Spec;
}

func (self *StringVessel) GetState() State	{ return self.state }

func (self *StringVessel) SetState(st State)	{ self.state = st }

func (self *StringVessel) GetInput() Input	{
    i := 0;
    for o, _ := range self.input {
        if i == self.position.Offset {
            return self.input[o:];
        }
        i++
    }

    return ""
}

func (self *StringVessel) Get(i int) (Input, bool) {
	if len(self.input) < self.position.Offset+i {
		return "", false
	}

    s := "";
    n := 0;
    for _, v := range self.input {
        if n >= self.position.Offset {
            if n > self.position.Offset + i {
                break
            }
            s += string(v);
        }
        n++
    }

    return s, true
}

func (self *StringVessel) Next() (int, bool) {
	if len(self.input) < self.position.Offset+1 {
		return 0, false
	}

    i := 0;
    for _, v := range self.input {
        if i == self.position.Offset {
            return int(v), true;
        }
        i++
    }

	return 0, false;
}

func (self *StringVessel) Pop(i int)	{ self.position.Offset += i }

func (self *StringVessel) Push(i int)	{ self.position.Offset -= i }

func (self *StringVessel) SetInput(in Input)	{ self.input = in.(string) }

func (self *StringVessel) GetPosition() Position {
	return self.position
}

func (self *StringVessel) SetPosition(pos Position) {
	self.position = pos
}

func (self *StringVessel) GetSpec() Spec	{ return self.spec }

func (self *StringVessel) SetSpec(sp Spec)	{ self.spec = sp }
