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
	// TODO maybe input init size of main array
	// TODO deal with multiline strings
	// TODO deal with codestyle
	write(".section .bss\n"+
		".lcomm cells, 30720         # allocating main array\n\n"+
		".section .text\n"+
		".global _start\n\n"+
		"_start:\n"+
		"    xor %r9, %r9            # init reg as zero\n"+
		"    xor %r10, %r10          # init reg as zero\n"+
		"    lea cells(%rip), %r10   # array ptr into rsi\n\n",
		out,
		OutPath)

	line := 1
	col := 0
	reader := bufio.NewReader(src)
	for {
		char, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				// TODO verbose output
				write("\n    mov $60, %rax           # syscall num for exit\n"+
					"    xor %rbx, %rbx          # clear exit code reg\n"+
					"    syscall\n",
					out, OutPath)
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

		// r9 for data pointer
		// r10 for main array pointer
		switch char {
		// TODO maybe substitute dynamical pointer computing with constants computing at compile time
		// TODO optimize many add/subs/shifts in a row
		case Plus:
			write("    addb $1, (%r10, %r9, 1) # increment data at array pointer + data pointer\n", out, OutPath)
		case Minus:
			write("    subb $1, (%r10, %r9, 1) # decrement data at array pointer + data pointer\n", out, OutPath)
		case RShift:
			write("    add $1, %r9w            # increment data pointer\n", out, OutPath)
		case LShift:
			write("    sub $1, %r9w            # decrement data pointer\n", out, OutPath)
		case Input:
			// TODO
		case Output:
			write("\n    mov $1, %rax # sys_write syscall\n"+
				"    mov $1, %rdi # stdout file descriptor\n"+
				"    lea (%r10, %r9, 1), %rsi # mov cur cell\n"+
				"    mov $1, %rdx # write 1 byte\n"+
				"    syscall\n",
				out, OutPath)
		case CycleStart:
			// TODO
		case CycleEnd:
			// TODO
		case Commentary:
			// TODO better skipping
			_, err := reader.ReadString('\n')
			if err != nil {
				panic(err)
			}
			line++
			col = 0
		case '\n':
			// TODO bandaid
		default:
			// TODO better formatting
			panic(SrcPath + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(col) + " ERROR: Unexcepted token: " + string(char)) // TODO string(rune)
		}
	}
}

func write(payload string, file *os.File, name string) {
	_, err := file.WriteString(payload)
	if err != nil {
		panic("ERROR: unable to write to the file " + name + ". " + err.Error())
	}
}
