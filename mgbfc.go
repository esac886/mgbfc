package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// TODO const naming
const (
	Rodata = ".section .rodata\n" +
		".set tape_size, %d\n\n"
	Bss = ".section .bss\n" +
		".lcomm tape, tape_size\n" +
		".lcomm in_buf, 1\n\n" // TODO maybe alloc input buf if necessary
	Text = ".section .text\n" +
		".global _start\n\n" +
		"_start:\n" +
		"    xor     %r9,            %r9\n" +
		"    xor     %r10,           %r10\n" +
		"    lea     tape(%rip),     %r10\n" +
		"    xor     %rsi,           %rsi\n"
	Epilogue = "    mov     $60,            %rax\n" +
		"    xor     %rbx,           %rbx\n" +
		"    syscall\n"
	Input = "    mov     $0,              %rax\n" +
		"    mov     $0,              %rdi\n" +
		"    mov     $1,              %rdx\n" +
		"    lea     in_buf(%rip),    %rsi\n" +
		"    syscall\n" +
		"    movb    (%rsi),          %r11b\n" +
		"    movb    %r11b,           (%r10, %r9, 1)\n"
	Output = "    mov     $1,             %rax\n" +
		"    mov     $1,             %rdi\n" +
		"    lea     (%r10, %r9, 1), %rsi\n" +
		"    mov     $1,             %rdx\n" +
		"    syscall\n"
	CycleStart = "\ns%c:\n" +
		"    cmpb    $0,             (%%r10, %%r9, 1)\n" +
		"    je      e%c\n"
	CycleEnd = "    cmpb    $0,             (%%r10, %%r9, 1)\n" +
		"    jg      s%c\n\n" +
		"e%c:\n"
	Increment       = "    addb    $1,             (%r10, %r9, 1)\n"
	Decrement       = "    subb    $1,             (%r10, %r9, 1)\n"
	IncrDataPtr     = "    add     $1,             %r9w\n"
	DecrDataPtr     = "    sub     $1,             %r9w\n"
	ResetDataPtr    = "    mov     $0,             %r9w\n"
	MaximizeDataPtr = "    mov     $tape_size,     %r9w\n"
)

var logger = log.New(os.Stderr, "", 0)

func main() {
	var TapeSize uint16 = 30720
	// TODO input filename and add flags for: help, output file name, verbose output
	SrcPath := "test.bf"

	src, err := os.Open(SrcPath)
	if err != nil {
		logger.Fatalf("ERROR: unable to read the file %s. %s\n", SrcPath, err.Error())
	}

	defer func() {
		if err := src.Close(); err != nil {
			logger.Fatalf("ERROR: unable to close the file %s. %s\n", SrcPath, err.Error())
		}
	}() // TODO what is this brackets

	// TODO better way to generate OutPath
	OutPath := strings.Split(SrcPath, ".")[0] + ".asm"
	out, err := os.Create(OutPath)
	if err != nil {
		logger.Fatalf("ERROR: unable to create the file %s. %s\n", OutPath, err.Error())
	}

	defer func() {
		if err := out.Close(); err != nil {
			logger.Fatalf("ERROR: unable to close the file %s. %s\n", OutPath, err.Error())
		}
	}() // TODO what is this brackets

	// TODO maybe input init size of main array
	write(fmt.Sprintf(Rodata, TapeSize), out, OutPath)
	write(Bss, out, OutPath)
	write(Text, out, OutPath)

	// TODO expand labels range. now it is between 65 (A) to 90 (Z)
	curLabel := -1
	var labelChar byte = 64 // A - 1
	var labels [25]byte
	var dataPtr uint16 = 0

	line := 1
	col := 0

	reader := bufio.NewReader(src)
	for {
		char, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				// TODO verbose output
				write(Epilogue, out, OutPath)
				break
			} else {
				logger.Fatalf("%s:%d:%d ERROR: %s\n", SrcPath, line, col, err.Error())
			}
		}

		// saving location
		if char == '\n' {
			line++
			col = 1
		} else {
			col++
		}

		// r9 for data pointer
		// r10 for main array pointer
		// r11 as intermediate reg
		switch char {
		// TODO optimize many add/subs/shifts in a row
		// TODO optimal string formatting
		case '+':
			write("    addb    $1,             (%r10, %r9, 1)\n", out, OutPath)
		case '-':
			write("    subb    $1,             (%r10, %r9, 1)\n", out, OutPath)
		case '>':
			if dataPtr == (TapeSize - 1) {
				dataPtr = 0
				write("    mov     $0,             %r9w\n", out, OutPath)
			} else {
				dataPtr++
				write("    add     $1,             %r9w\n", out, OutPath)
			}
		case '<':
			if dataPtr == 0 {
				dataPtr = TapeSize - 1
				write("    mov     $tape_size,     %r9w\n", out, OutPath)
			} else {
				dataPtr--
				write("    sub     $1,             %r9w\n", out, OutPath)
			}
		case ',':
			write(Input, out, OutPath)
		case '.':
			write(Output, out, OutPath)
		case '[':
			// TODO brackets check
			labelChar++
			curLabel++
			labels[curLabel] = labelChar
			write(fmt.Sprintf(CycleStart, labelChar, labelChar), out, OutPath)
		case ']':
			write(fmt.Sprintf(CycleEnd, labels[curLabel], labels[curLabel]), out, OutPath)
			curLabel--
		case '#':
			// TODO better skipping
			_, err := reader.ReadString('\n')
			if err != nil {
				panic(err)
			}
			line++
			col = 0
		case '\n', ' ':
			// TODO it's a bandaid
		default:
			logger.Fatalf("%s:%d:%d ERROR: Unexcepted token: %c\n", SrcPath, line, col, char)
		}
	}
}

func write(payload string, file *os.File, name string) {
	if _, err := file.WriteString(payload); err != nil {
		logger.Fatalf("ERROR: Unable to write to the file %s. %s\n", name, err.Error())
	}
}
