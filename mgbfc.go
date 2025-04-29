package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	rodata = `.section .rodata
.set tape_size, %d
`

	bss = `.section .bss
.lcomm tape, tape_size
.lcomm in_buf, 1
` // TODO maybe alloc input buf if necessary

	text = `.section .text
.global _start

_start:
    xor     %r9,            %r9
    xor     %r10,           %r10
    lea     tape(%rip),     %r10
    xor     %rsi,           %rsi
`

	epilogue = `    mov     $60,            %rax
    xor     %rbx,           %rbx
    syscall
`

	input = `    mov     $0,              %rax
    mov     $0,              %rdi
    mov     $1,              %rdx
    lea     in_buf(%rip),    %rsi
    syscall
    movb    (%rsi),          %r11b
    movb    %r11b,           (%r10, %r9, 1)
`

	output = `    mov     $1,             %rax
    mov     $1,             %rdi
    lea     (%r10, %r9, 1), %rsi
    mov     $1,             %rdx
    syscall
`

	cycleStart = `
s%d:
    cmpb    $0,             (%%r10, %%r9, 1)
    je      e%d
`

	cycleEnd = `    cmpb    $0,             (%%r10, %%r9, 1)
    jg      s%d

e%d:
`

	increment       = "    addb    $1,             (%r10, %r9, 1)\n"
	decrement       = "    subb    $1,             (%r10, %r9, 1)\n"
	incrDataPtr     = "    add     $1,             %r9w\n"
	decrDataPtr     = "    sub     $1,             %r9w\n"
	resetDataPtr    = "    mov     $0,             %r9w\n"
	maximizeDataPtr = "    mov     $tape_size,     %r9w\n"

	tapeSizeMax     = 4_294_967_296 // 4 GiB
	tapeSizeDefault = 30720
)

var logger = log.New(os.Stderr, "", 0)
var verbose bool

func main() {
	// flags
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage:\n\t%s [options] [file1 file2 ... fileN]\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	var outPath string
	flag.StringVar(&outPath, "o", "", "Specify name of the output executable (source code file name without .bf extension by default).\n"+
		"If more than one source code file is provided, then this flag will be ignored, and output executables will have the default name.")

	var genAsm, genObject bool
	flag.BoolVar(&verbose, "v", false, "Enable verbose output.")
	flag.BoolVar(&genAsm, "S", false, "Generate .s file with GNU assembly translation of provided .bf source code.")
	flag.BoolVar(&genObject, "c", false, "Generate .o file with GNU assembly translation of provided .bf source code.")

	var tapeSize uint
	flag.UintVar(&tapeSize, "s", tapeSizeDefault, "Defines the size of a tape of Turing machine in Kb. Must be above zero.")

	flag.Parse()

	if tapeSize == 1 {
		logger.Println("INFO: provided tape size is below one. The value of the tape size will be assigned to one.")
		tapeSize = 1
	} else if tapeSize > tapeSizeMax {
		logger.Println("INFO: provided tape size is bigger than the maximum value (4 GiB). The value of the tape size will be substracted to maximum.")
		tapeSize = tapeSizeMax
	}

	// processing source code files
	sourceFiles := flag.Args()
	if len(sourceFiles) == 0 {
		fmt.Println("Nothing to do. Try 'mgbfc -h'")
		return
	}

	for _, srcPath := range sourceFiles {
		srcFile := openFile(srcPath)
		defer closeFile(srcFile)

		// generating out file name if not provided, or if source files more than one
		if outPath == "" || len(sourceFiles) > 1 {
			ex, err := os.Executable()
			if err != nil {
				logger.Fatalln("ERROR: unable to get path name for the compiler executable")
			}

			// outPath = current working dir for compiler + name of .bf file without .bf extension
			if !strings.Contains(srcPath, ".bf") {
				logger.Printf("ERROR: provided file %s does not have .bf extension. Ignoring this file. Please provide source code files only with .bf extension\n", srcPath)
				continue
			}

			baseSrc := filepath.Base(srcPath)
			outPath = filepath.Dir(ex) + "/" + baseSrc[:len(baseSrc)-3]
		}

		tmpAsm, err := os.CreateTemp("", "mgbfc-*.s")
		if err != nil {
			logger.Fatalf("ERROR: unable to create the temporary assembly file %s.\n", tmpAsm.Name())
		}
		defer removeFile(tmpAsm.Name())

		// TODO why did it compile without error handling
		// defer os.Remove(tmpAsm.Name()) // clean up file afterwards

		//generating prologue
		writeFile(fmt.Sprintf(rodata, tapeSize), tmpAsm)
		writeFile(bss, tmpAsm)
		writeFile(text, tmpAsm)

		var dataPtr uint = 0

		// location
		line := 1
		col := 0

		labelIdx := -1
		label := -1
		labels := make([]uint, 25)

		// reading the bf code file and generating the asm
		reader := bufio.NewReader(srcFile)
		for {
			char, _, err := reader.ReadRune()
			if err != nil {
				if err == io.EOF {
					// TODO verbose output
					writeFile(epilogue, tmpAsm)
					break
				} else {
					logger.Fatalf("%s:%d:%d ERROR: %s\n", srcPath, line, col, err.Error())
				}
			}

			// r9 for data pointer
			// r10 for main array pointer
			// r11 as intermediate reg
			switch char {
			// TODO optimize many add/subs/shifts in a row
			// TODO optimal string formatting
			case '+':
				writeFile("    addb    $1,             (%r10, %r9, 1)\n", tmpAsm)
			case '-':
				writeFile("    subb    $1,             (%r10, %r9, 1)\n", tmpAsm)
			case '>':
				if dataPtr == (tapeSize - 1) {
					dataPtr = 0
					writeFile("    mov     $0,             %r9w\n", tmpAsm)
				} else {
					dataPtr++
					writeFile("    add     $1,             %r9w\n", tmpAsm)
				}
			case '<':
				if dataPtr == 0 {
					dataPtr = tapeSize - 1
					writeFile("    mov     $tape_size,     %r9w\n", tmpAsm)
				} else {
					dataPtr--
					writeFile("    sub     $1,             %r9w\n", tmpAsm)
				}
			case ',':
				writeFile(input, tmpAsm)
			case '.':
				writeFile(output, tmpAsm)
			case '[':
				// TODO brackets check
				label++
				labelIdx++
				labels[labelIdx] = uint(label)
				writeFile(fmt.Sprintf(cycleStart, label, label), tmpAsm)
			case ']':
				writeFile(fmt.Sprintf(cycleEnd, labels[labelIdx], labels[labelIdx]), tmpAsm)
				labelIdx--
			case '#':
				// TODO better skipping
				_, err := reader.ReadString('\n')
				if err != nil {
					panic(err)
				}
				fallthrough
			case '\n':
				line++
				col = 0
			case ' ':
			default:
				logger.Fatalf("%s:%d:%d: ERROR: Unexcepted token: %c\n", srcPath, line, col, char)
			}

			col++
		}

		// closing tmp assembly file before generating output file
		tmpAsmPath := tmpAsm.Name()
		closeFile(tmpAsm)

		// generating tmp object file
		if !genAsm {
			tmpFilePath := tmpAsmPath[:len(tmpAsmPath)-2]
			tmpObjPath := tmpFilePath + ".o"

			// generating the object file from the generated asm
			// TODO maybe add fasm
			executeCommand("as", tmpAsmPath, "-o", tmpObjPath)
			defer removeFile(tmpObjPath)

			if !genObject {
				// generating the executable from the generated object file
				// TODO I can generate ELF64 header by myself https://tuket.github.io/notes/asm/elf64_hello_world
				executeCommand("ld", tmpObjPath, "-o", outPath)
			} else {
				// generating output object file
				outObj := createFile(outPath + ".o")
				defer closeFile(outObj)

				tmpObj := openFile(tmpObjPath)
				defer closeFile(tmpObj)

				copy(outObj, tmpObj)
			}
		} else {
			// generating out assembly file
			outAsm := createFile(outPath + ".s")
			defer closeFile(outAsm)

			tmpAsm := openFile(tmpAsmPath)
			defer closeFile(tmpAsm)

			copy(outAsm, tmpAsm)
		}
	}
}

func createFile(path string) *os.File {
	if verbose {
		logger.Printf("Creating '%s'\n", path)
	}
	file, err := os.Create(path)
	if err != nil {
		// TODO if file creating was failed, does Name() method returns valid??
		logger.Fatalf("ERROR: failed to create file '%s'. %s.\n", file.Name(), err.Error())
	}
	return file
}

func writeFile(payload string, file *os.File) {
	_, err := file.WriteString(payload)
	if err != nil {
		logger.Fatalf("ERROR: Unable to write to the file '%s'. %s\n", file.Name(), err.Error())
	}
}

func openFile(path string) *os.File {
	if verbose {
		logger.Printf("Opening '%s'\n", path)
	}
	file, err := os.Open(path)
	if err != nil {
		logger.Fatalf("ERROR: unable to read the file '%s'. %s.\n", path, err.Error())
	}
	return file
}

func closeFile(file *os.File) {
	if verbose {
		logger.Printf("Closing '%s'\n", file.Name())
	}
	if err := file.Close(); err != nil {
		logger.Fatalf("ERROR: unable to close the file '%s'. %s.\n", file.Name(), err.Error())
	}
}

func removeFile(path string) {
	if verbose {
		logger.Printf("Removing '%s'\n", path)
	}
	if err := os.Remove(path); err != nil {
		logger.Fatalf("ERROR: unable to remove the file '%s'. %s\n", path, err.Error())
	}
}

func copy(target *os.File, source *os.File) {
	if verbose {
		logger.Printf("Copying '%s' to '%s'\n", target.Name(), source.Name())
	}
	_, err := io.Copy(target, source)
	if err != nil {
		logger.Fatalf("ERROR: failed to copy '%s' contents to '%s'. %s\n", source.Name(), target.Name(), err.Error())
	}
}

func executeCommand(name string, args ...string) {
	if verbose {
		logger.Printf("Executing '%s %s'\n", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		logger.Fatalf("ERROR: failed to generate object file with as. %s\n", err.Error())
	}
}
