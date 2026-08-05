package gamespy

import (
	"bytes"
)

// ParseGameSpyMessage splits wire data into complete commands and leftover bytes.
func ParseGameSpyMessage(data []byte) (commands []map[string]string, remainder []byte) {
	msg := data
	finalMarker := []byte("\\final\\")

	for len(msg) > 0 && msg[0] == '\\' && bytes.Contains(msg, finalMarker) {
		messages := make(map[string]string)
		foundCommand := false

		for len(msg) > 0 && msg[0] == '\\' {
			keyEnd := bytes.IndexByte(msg[1:], '\\') + 1
			if keyEnd < 1 {
				break
			}
			key := string(msg[1:keyEnd])
			msg = msg[keyEnd+1:]

			if key == "final" {
				break
			}

			var value string
			if len(msg) == 0 {
				value = ""
			} else if msg[0] == '\\' {
				value = ""
			} else {
				valueEnd := bytes.IndexByte(msg[1:], '\\')
				if valueEnd < 0 {
					value = string(msg)
					msg = nil
				} else {
					value = string(msg[:valueEnd+1])
					msg = msg[valueEnd+1:]
				}
			}

			if !foundCommand {
				messages["__cmd__"] = key
				messages["__cmd_val__"] = value
				foundCommand = true
			}
			messages[key] = value
		}

		commands = append(commands, messages)
	}

	if len(msg) > 0 {
		remainder = msg
	}
	return commands, remainder
}

// CreateGameSpyMessage builds a GameSpy wire message ending with \final\.
func CreateGameSpyMessage(fields map[string]string) []byte {
	cmd := stripBackslash(fields["__cmd__"])
	cmdVal := stripBackslash(fields["__cmd_val__"])

	var buf []byte
	for key, value := range fields {
		if key == cmd || key == "__cmd__" || key == "__cmd_val__" {
			continue
		}
		buf = append(buf, '\\')
		buf = append(buf, stripBackslash(key)...)
		buf = append(buf, '\\')
		buf = append(buf, stripBackslash(value)...)
	}

	if cmd != "" {
		prefix := []byte{'\\'}
		prefix = append(prefix, cmd...)
		prefix = append(prefix, '\\')
		prefix = append(prefix, cmdVal...)
		buf = append(prefix, buf...)
	}

	buf = append(buf, "\\final\\"...)
	return buf
}

// KV is an ordered GameSpy wire field.
type KV struct {
	Key   string
	Value string
}

// CreateGameSpyMessageOrdered builds \key\value...\\final\ in pair order.
// Callers should put the command as the first pair (key = command name, value often empty).
func CreateGameSpyMessageOrdered(pairs []KV) []byte {
	var buf []byte
	for _, pair := range pairs {
		buf = append(buf, '\\')
		buf = append(buf, stripBackslash(pair.Key)...)
		buf = append(buf, '\\')
		buf = append(buf, stripBackslash(pair.Value)...)
	}
	buf = append(buf, "\\final\\"...)
	return buf
}

func stripBackslash(s string) string {
	return stringsTrimBackslash(s)
}

func stringsTrimBackslash(s string) string {
	for len(s) > 0 && s[0] == '\\' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '\\' {
		s = s[:len(s)-1]
	}
	return s
}
