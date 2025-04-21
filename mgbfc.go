package main

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// tokens
const (
	Plus       = '+'
	Minus      = '-'
	RShift     = '>'
	LShift     = '<'
	Input      = ','
	Output     = '.'
	CycleStart = '['
	CycleEnd   = ']'
	Commentary = '#'
)

func main() {
	// TODO input filename and add flags for: help, output file name, verbose output
	SrcPath := "test.bf"

	src, err := os.Open(SrcPath)
	if err != nil {
		// TODO better err handling
		panic("ERROR: unable to read the file " + SrcPath + ". " + err.Error())
	}

	defer func() {
		if err := src.Close(); err != nil {
			panic("ERROR: unable to close the file " + SrcPath + ". " + err.Error())
		}
	}() // TODO what is this brackets

	// TODO better way to generate OutPath
	OutPath := strings.Split(SrcPath, ".")[0] + ".asm"
	out, err := os.Create(OutPath)
	if err != nil {
		panic("ERROR: unable to create the file " + OutPath + ". " + err.Error())
	}

	defer func() {
		if err := out.Close(); err != nil {
			panic("ERROR: unable to close the file " + OutPath + ". " + err.Error())
		}
	}() // TODO what is this brackets

	// prologue
	_, err = out.WriteString(".section .text\n.global _start\n_start:")
	if err != nil {
		panic("ERROR: unable to write to the file " + OutPath + ". " + err.Error())
	}

	line := 1
	col := 0
	reader := bufio.NewReader(src)
	for {
		char, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				// TODO verbose output
				break
			} else {
				panic(SrcPath + " ERROR: " + err.Error())
			}
		}

		// saving location
		if char == '\n' {
			line++
			col = 0
		} else {
			col++
		}

		switch char {
		case Plus:
		case Minus:
		case RShift:
		case LShift:
		case Input:
		case Output:
		case CycleStart:
		case CycleEnd:
		case Commentary:
			// TODO better skipping
			_, err := reader.ReadString('\n')
			if err != nil {
				panic(err)
			}
			line++
			col = 0
		default:
			// TODO better formatting
			panic(SrcPath + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(col) + " ERROR: Unexcepted token: " + string(char)) // TODO string(rune)
		}
	}
}
