package main

import (
	"bufio"
	"flag"
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

// TODO generate asm in tmp file and pass to as
// TODO generate obj in tmp file and pass to ld
func main() {
	// flags

	// TODO help flag
	var description string

	description = "Defines the size of a tape of Turing machine in Kb (30 Kb by default)"
	// TODO maybe uint32
	var tapeSize uint
	flag.UintVar(&tapeSize, "s", 30*1024, description)
	flag.UintVar(&tapeSize, "tape-size", 30*1024, description)
	// TODO tapeSize limitation

	description = "Specify name of the output executable (source code file name without .bf extension by default). " +
		"If more than one source code file is provided, then this flag will be ignored, and output executables will have a default name."
	var outPath string
	flag.StringVar(&outPath, "o", "", description)
	flag.StringVar(&outPath, "out", "", description)

	var genAsm bool
	description = "Generate .s file with GNU assembly translation of provided .bf source code."
	flag.BoolVar(&genAsm, "S", false, description)
	flag.BoolVar(&genAsm, "gen-asm", false, description)

	var genObject bool
	description = "Generate .o file with GNU assembly translation of provided .bf source code."
	flag.BoolVar(&genObject, "c", false, description)
	flag.BoolVar(&genObject, "gen-obj", false, description)

	flag.Parse()

	// TODO autosearch for all .bf files in PWD if not specified any filenames
	// processing source code files
	sourceFiles := flag.Args()
	for _, srcPath := range sourceFiles {
		srcFile, err := os.Open(srcPath)
		if err != nil {
			// TODO better error handling
			logger.Fatalf("ERROR: unable to read the file %s. %s\n", srcPath, err.Error())
		}

		defer func() {
			if err := srcFile.Close(); err != nil {
				logger.Fatalf("ERROR: unable to close the file %s. %s\n", srcPath, err.Error())
			}
		}() // TODO what is this brackets

		if outPath == "" || len(sourceFiles) > 1 {
			// TODO better way to generate the outPath
			outPath = strings.Split(srcPath, ".")[0]
		}

		// creating an asm and an object file if the flags is provided
		if genAsm {
			// TODO
		}
		if genObject {
			// TODO
		}

		outFile, err := os.Create(outPath)
		if err != nil {
			logger.Fatalf("ERROR: unable to create the file %s. %s\n", outPath, err.Error())
		}
		defer func() {
			if err := outFile.Close(); err != nil {
				logger.Fatalf("ERROR: unable to close the file %s. %s\n", outPath, err.Error())
			}
		}() // TODO what is this brackets

		// generating prologue
		write(fmt.Sprintf(Rodata, tapeSize), outFile, outPath)
		write(Bss, outFile, outPath)
		write(Text, outFile, outPath)

		// TODO expand labels range. now it is between 65 (A) to 90 (Z)
		curLabel := -1
		var labelChar byte = 64 // A - 1
		var labels [25]byte
		var dataPtr uint = 0

		line := 1
		col := 0

		reader := bufio.NewReader(srcFile)
		for {
			char, _, err := reader.ReadRune()
			if err != nil {
				if err == io.EOF {
					// TODO verbose output
					write(Epilogue, outFile, outPath)
					break
				} else {
					logger.Fatalf("%s:%d:%d ERROR: %s\n", srcPath, line, col, err.Error())
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
				write("    addb    $1,             (%r10, %r9, 1)\n", outFile, outPath)
			case '-':
				write("    subb    $1,             (%r10, %r9, 1)\n", outFile, outPath)
			case '>':
				if dataPtr == (tapeSize - 1) {
					dataPtr = 0
					write("    mov     $0,             %r9w\n", outFile, outPath)
				} else {
					dataPtr++
					write("    add     $1,             %r9w\n", outFile, outPath)
				}
			case '<':
				if dataPtr == 0 {
					dataPtr = tapeSize - 1
					write("    mov     $tape_size,     %r9w\n", outFile, outPath)
				} else {
					dataPtr--
					write("    sub     $1,             %r9w\n", outFile, outPath)
				}
			case ',':
				write(Input, outFile, outPath)
			case '.':
				write(Output, outFile, outPath)
			case '[':
				// TODO brackets check
				labelChar++
				curLabel++
				labels[curLabel] = labelChar
				write(fmt.Sprintf(CycleStart, labelChar, labelChar), outFile, outPath)
			case ']':
				write(fmt.Sprintf(CycleEnd, labels[curLabel], labels[curLabel]), outFile, outPath)
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
				logger.Fatalf("%s:%d:%d ERROR: Unexcepted token: %c\n", srcPath, line, col, char)
			}
		}
	}

}

func write(payload string, file *os.File, name string) {
	if _, err := file.WriteString(payload); err != nil {
		logger.Fatalf("ERROR: Unable to write to the file %s. %s\n", name, err.Error())
	}
}
