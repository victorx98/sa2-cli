package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/term"
)

const maxPasswordSize = 64 * 1024

var (
	errPassword     = errors.New("invalid password input")
	errPasswordMode = errors.New("unsupported terminal input mode")
)

func (a App) password(ctx context.Context, fromStdin bool) ([]byte, error) {
	inputTerminal := term.IsTerminal(int(a.In.Fd()))
	if fromStdin {
		if inputTerminal {
			return nil, errPasswordMode
		}
		value, err := io.ReadAll(io.LimitReader(a.In, maxPasswordSize+3))
		if err != nil {
			clear(value)
			return nil, errPassword
		}
		trimmed := bytes.TrimSuffix(value, []byte("\r\n"))
		if len(trimmed) == len(value) {
			trimmed = bytes.TrimSuffix(value, []byte("\n"))
		}
		return validatePassword(trimmed)
	}
	if !inputTerminal || !term.IsTerminal(int(a.Err.Fd())) {
		return nil, errPasswordMode
	}
	return a.terminalPassword(ctx)
}

func (a App) terminalPassword(ctx context.Context) (value []byte, resultErr error) {
	fd := int(a.In.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := term.Restore(fd, state); err != nil {
			clear(value)
			value, resultErr = nil, err
		}
		fmt.Fprintln(a.Err)
	}()
	if _, err := fmt.Fprint(a.Err, "Password: "); err != nil {
		return nil, err
	}

	type result struct {
		value []byte
		err   error
	}
	done := make(chan result)
	go func() {
		value, err := readTerminalLine(a.In)
		select {
		case done <- result{value, err}:
		case <-ctx.Done():
			clear(value)
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		return result.value, result.err
	}
}

func readTerminalLine(in *os.File) ([]byte, error) {
	var value []byte
	var input [1]byte
	for {
		if _, err := io.ReadFull(in, input[:]); err != nil {
			clear(value)
			return nil, errPassword
		}
		switch input[0] {
		case '\r', '\n':
			return validatePassword(value)
		case 3:
			clear(value)
			return nil, context.Canceled
		case 4:
			clear(value)
			return nil, errPassword
		case 8, 127:
			if _, size := utf8.DecodeLastRune(value); size != 0 {
				cursor := len(value) - size
				clear(value[cursor:])
				value = value[:cursor]
			}
		default:
			if len(value) == maxPasswordSize {
				clear(value)
				return nil, errPassword
			}
			value = append(value, input[0])
		}
	}
}

func validatePassword(value []byte) ([]byte, error) {
	if len(value) == 0 || len(value) > maxPasswordSize || !utf8.Valid(value) {
		clear(value)
		return nil, errPassword
	}
	return value, nil
}
