package render770

// signable.go reads the command tree the world sends (the Commands packet
// body) for the arguments a client signs: every minecraft:message argument
// (vanilla SignedArgument — /say's message, /me's action, /msg's message,
// /teammsg's message). The gateway holds the player's message chain, so it
// is the gateway that checks a chat_command_signed's argument signatures,
// and it needs to know, per command, which arguments those are
// (SignableCommand.of over the parse).
//
// The tree carries 26.x argument-type ids (the world writes only ids that
// 26.2 and 26.3 share; see the engine's command tree).

import (
	"bytes"
	"io"
	"strings"

	"github.com/tachyne/tachyne-common/protocol"
)

// ParserMessage is minecraft:message's argument-type id on 26.2 and 26.3
// (ArgumentTypeInfos registration order).
const ParserMessage = 20

// SignableArg is one minecraft:message argument under a command: its node
// name (the name the client signs it under) and Depth, the number of
// argument and literal nodes from the command's literal to it, inclusive —
// the fewest space-separated words that must follow the command's name for
// a parse to reach it.
type SignableArg struct {
	Name  string
	Depth int
}

// SignableIndex maps a root command literal to the message arguments under
// it (following redirects). Commands with none are absent.
type SignableIndex map[string][]SignableArg

// treeNode is one Commands-packet node, as far as signing needs it.
type treeNode struct {
	kind     byte // 0 root, 1 literal, 2 argument
	children []int32
	redirect int32 // -1 = none
	name     string
	parser   int32
}

// skipParserProperties advances past one argument node's properties
// (ArgumentTypeInfo.serializeToNetwork for the 26.x ids that write any).
func skipParserProperties(r *bytes.Reader, parser int32) error {
	skip := func(n int) error {
		if r.Len() < n {
			return io.ErrUnexpectedEOF
		}
		_, err := r.Seek(int64(n), io.SeekCurrent)
		return err
	}
	switch parser {
	case 1, 2, 3, 4: // brigadier float, double, integer, long: flags, then min/max
		flags, err := r.ReadByte()
		if err != nil {
			return err
		}
		width := map[int32]int{1: 4, 2: 8, 3: 4, 4: 8}[parser]
		if flags&1 != 0 {
			if err := skip(width); err != nil {
				return err
			}
		}
		if flags&2 != 0 {
			return skip(width)
		}
		return nil
	case 5: // brigadier:string: the StringType as a VarInt
		_, err := protocol.ReadVarInt(r)
		return err
	case 6, 31: // entity, score_holder: a flags byte
		_, err := r.ReadByte()
		return err
	case 43: // time: the minimum as an int
		return skip(4)
	case 44, 45, 46, 47, 48: // resource_or_tag(_key), resource(_key), resource_selector: the registry key
		_, err := protocol.ReadString(r)
		return err
	}
	return nil // every other type is a singleton with no properties
}

// parseTree reads a Commands packet body into nodes and the root index.
func parseTree(tree []byte) ([]treeNode, int32, bool) {
	r := bytes.NewReader(tree)
	n, err := protocol.ReadVarInt(r)
	if err != nil || n <= 0 || n > 1<<16 {
		return nil, 0, false
	}
	nodes := make([]treeNode, n)
	for i := range nodes {
		flags, err := r.ReadByte()
		if err != nil {
			return nil, 0, false
		}
		nd := &nodes[i]
		nd.kind = flags & 3
		nd.redirect = -1
		c, err := protocol.ReadVarInt(r)
		if err != nil || c < 0 || int(c) > r.Len() {
			return nil, 0, false
		}
		for j := int32(0); j < c; j++ {
			idx, err := protocol.ReadVarInt(r)
			if err != nil || idx < 0 || idx >= n {
				return nil, 0, false
			}
			nd.children = append(nd.children, idx)
		}
		if flags&0x08 != 0 {
			red, err := protocol.ReadVarInt(r)
			if err != nil || red < 0 || red >= n {
				return nil, 0, false
			}
			nd.redirect = red
		}
		if nd.kind == 1 || nd.kind == 2 {
			if nd.name, err = protocol.ReadString(r); err != nil {
				return nil, 0, false
			}
		}
		if nd.kind == 2 {
			if nd.parser, err = protocol.ReadVarInt(r); err != nil {
				return nil, 0, false
			}
			if err := skipParserProperties(r, nd.parser); err != nil {
				return nil, 0, false
			}
			if flags&0x10 != 0 { // custom suggestions
				if _, err := protocol.ReadString(r); err != nil {
					return nil, 0, false
				}
			}
		}
	}
	root, err := protocol.ReadVarInt(r)
	if err != nil || root < 0 || root >= n || r.Len() != 0 {
		return nil, 0, false
	}
	return nodes, root, true
}

// maxSignableDepth bounds the walk below one command.
const maxSignableDepth = 16

// SignableArguments indexes a Commands packet body's message arguments by
// command. ok is false when the body cannot be read (an argument type whose
// properties are unknown), in which case nothing is signable.
func SignableArguments(tree []byte) (SignableIndex, bool) {
	nodes, root, ok := parseTree(tree)
	if !ok {
		return nil, false
	}
	idx := SignableIndex{}
	for _, c := range nodes[root].children {
		cmd := nodes[c]
		if cmd.kind != 1 {
			continue
		}
		var found []SignableArg
		seen := map[string]bool{}
		var walk func(n int32, depth int)
		walk = func(n int32, depth int) {
			if depth > maxSignableDepth {
				return
			}
			nd := nodes[n]
			kids := nd.children
			// A redirect hands the parse to another node's children (tell →
			// msg); one back to the root (execute run) starts a new command,
			// whose arguments belong to that command, not this one.
			if nd.redirect >= 0 && nd.redirect != root {
				kids = nodes[nd.redirect].children
			}
			for _, k := range kids {
				kn := nodes[k]
				if kn.kind == 2 && kn.parser == ParserMessage {
					if !seen[kn.name] {
						seen[kn.name] = true
						found = append(found, SignableArg{Name: kn.name, Depth: depth + 1})
					}
					continue // a message reads the rest of the line
				}
				walk(k, depth+1)
			}
		}
		walk(c, 0)
		if len(found) > 0 {
			idx[cmd.name] = found
		}
	}
	return idx, true
}

// For returns the message arguments a command line can reach: those under
// its first word with at least Depth words after it.
func (x SignableIndex) For(command string) []SignableArg {
	if len(x) == 0 {
		return nil
	}
	head, rest, _ := strings.Cut(command, " ")
	args := x[head]
	if len(args) == 0 {
		return nil
	}
	words := 0
	if rest != "" {
		words = len(strings.Split(rest, " "))
	}
	var out []SignableArg
	for _, a := range args {
		if words >= a.Depth {
			out = append(out, a)
		}
	}
	return out
}

// MessageCandidates are the values a greedy message argument can have in a
// command line: every suffix that starts right after a space past the
// command's name, earliest first. A message argument reads the rest of the
// line (MessageArgument.parseText), so its signed value is one of these.
func MessageCandidates(command string) []string {
	var out []string
	first := strings.IndexByte(command, ' ')
	if first < 0 {
		return nil
	}
	for i := first; i < len(command); i++ {
		if command[i] == ' ' {
			out = append(out, command[i+1:])
		}
	}
	return out
}
