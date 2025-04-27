package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

const (
	rodata = `.section .rodata
.set tape_size, %d`

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
    xor     %rsi,           %rsi`

	epilogue = `    mov     $60,            %rax
    xor     %rbx,           %rbx
    syscall`

	input = `    mov     $0,              %rax
    mov     $0,              %rdi
    mov     $1,              %rdx
    lea     in_buf(%rip),    %rsi
    syscall
    movb    (%rsi),          %r11b
    movb    %r11b,           (%r10, %r9, 1)`

	output = `    mov     $1,             %rax
    mov     $1,             %rdi
    lea     (%r10, %r9, 1), %rsi
    mov     $1,             %rdx
    syscall`

	cycleStart = `
%c:
    cmpb    $0,             (%%r10, %%r9, 1)
    je      e%c`

	cycleEnd = `    cmpb    $0,             (%%r10, %%r9, 1)
    jg      s%c

e%c:`

	increment       = "    addb    $1,             (%r10, %r9, 1)\n"
	decrement       = "    subb    $1,             (%r10, %r9, 1)\n"
	incrDataPtr     = "    add     $1,             %r9w\n"
	decrDataPtr     = "    sub     $1,             %r9w\n"
	resetDataPtr    = "    mov     $0,             %r9w\n"
	maximizeDataPtr = "    mov     $tape_size,     %r9w\n"
)

var logger = log.New(os.Stderr, "", 0)

func main() {
	// flags
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage:\n\t%s [options] [file1 file2 ... fileN]\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	var outPath string
	flag.StringVar(&outPath, "o", "", "Specify name of the output executable (source code file name without .bf extension by default).\n"+
		"If more than one source code file is provided, then this flag will be ignored, and output executables will have the default name.")

	var genAsm, genObject, verbose bool
	flag.BoolVar(&verbose, "v", false, "Enable verbose output (such as executed commands and etc).")
	flag.BoolVar(&genAsm, "S", false, "Generate .s file with GNU assembly translation of provided .bf source code.")
	flag.BoolVar(&genObject, "c", false, "Generate .o file with GNU assembly translation of provided .bf source code.")

	// TODO tapeSize limitation
	// TODO maybe uint32
	var tapeSize uint
	flag.UintVar(&tapeSize, "s", 30*1024, "Defines the size of a tape of Turing machine in Kb.")

	flag.Parse()

	// processing source code files
	sourceFiles := flag.Args()
	if len(sourceFiles) == 0 {
		// TODO better usage
		fmt.Println("Nothing to do. Try 'mgbfc -h'")
		return
	}

	for _, srcPath := range sourceFiles {
		// TODO maybe shorten the open/close file parts
		srcFile, err := os.Open(srcPath)
		if err != nil {
			// TODO better error handling
			logger.Fatalf("ERROR: unable to read the source code file %s. %s.\n", srcPath, err.Error())
		}
		defer func() {
			if err := srcFile.Close(); err != nil {
				logger.Fatalf("ERROR: unable to close the source code file %s. %s.\n", srcPath, err.Error())
			}
		}() // TODO what is this brackets

		if outPath == "" || len(sourceFiles) > 1 {
			// TODO check if .bf extension is present in input source file
			// cut of .bf extension
			outPath = srcPath[:len(srcPath)-3]
		}

		// TODO maybe a better filename generation
		tmpAsm, err := os.CreateTemp("", "mgbfc-*.s") // "" = system temp dir
		if err != nil {
			logger.Fatal("ERROR: unable to create the temporary assembly file ", outPath, ".\n")
		}
		defer func() {
			if err := os.Remove(tmpAsm.Name()); err != nil {
				logger.Fatal("ERROR: unable to remove the temporary assembly file ", outPath, ".\n")
			}
		}()
		// TODO why did it compile without error handling
		//defer os.Remove(tmpAsm.Name()) // clean up file afterwards

		// generating prologue
		// TODO struct method to get rid of name and path in every func call
		write(fmt.Sprintf(rodata, tapeSize), tmpAsm)
		write(bss, tmpAsm)
		write(text, tmpAsm)

		// TODO expand labels range. now it is between 65 (A) to 90 (Z)
		curLabel := -1
		var labelChar byte = 64 // A - 1
		var labels [25]byte
		var dataPtr uint = 0

		line := 1
		col := 0

		// reading the bf code file and generating the asm
		reader := bufio.NewReader(srcFile)
		for {
			char, _, err := reader.ReadRune()
			if err != nil {
				if err == io.EOF {
					// TODO verbose output
					write(epilogue, tmpAsm)
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
				write("    addb    $1,             (%r10, %r9, 1)\n", tmpAsm)
			case '-':
				write("    subb    $1,             (%r10, %r9, 1)\n", tmpAsm)
			case '>':
				if dataPtr == (tapeSize - 1) {
					dataPtr = 0
					write("    mov     $0,             %r9w\n", tmpAsm)
				} else {
					dataPtr++
					write("    add     $1,             %r9w\n", tmpAsm)
				}
			case '<':
				if dataPtr == 0 {
					dataPtr = tapeSize - 1
					write("    mov     $tape_size,     %r9w\n", tmpAsm)
				} else {
					dataPtr--
					write("    sub     $1,             %r9w\n", tmpAsm)
				}
			case ',':
				write(input, tmpAsm)
			case '.':
				write(output, tmpAsm)
			case '[':
				// TODO brackets check
				labelChar++
				curLabel++
				labels[curLabel] = labelChar
				write(fmt.Sprintf(cycleStart, labelChar, labelChar), tmpAsm)
			case ']':
				write(fmt.Sprintf(cycleEnd, labels[curLabel], labels[curLabel]), tmpAsm)
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

		if !genAsm {
			tmpAsmPath := tmpAsm.Name()
			tmpFilePath := tmpAsmPath[:len(tmpAsmPath)-2]
			tmpObjPath := tmpFilePath + ".o"

			// generating the object file from the generated asm
			// TODO maybe add fasm
			as := exec.Command("as", tmpAsmPath, "-o", tmpObjPath)
			if verbose {
				logger.Print("as ", tmpAsmPath, " -o ", tmpObjPath)
			}
			as.Stdout = os.Stdout
			as.Stderr = os.Stderr
			// remove tmp object file after all
			defer func() {
				if err := os.Remove(tmpObjPath); err != nil {
					logger.Fatal("ERROR: failed to remove temporary object file ", tmpObjPath, ". ", err.Error(), ".\n")
				}
			}()

			err = as.Run()
			if err != nil {
				logger.Fatal("ERROR: failed to generate object file with as. ", err.Error(), ".\n")
			}

			if !genObject {
				// generating the executable from the generated object file
				// TODO maybe get rid of ld somehow
				ld := exec.Command("ld", tmpObjPath, "-o", outPath)
				if verbose {
					logger.Print("ld ", tmpObjPath, " -o ", outPath)
				}
				ld.Stdout = os.Stdout
				ld.Stderr = os.Stderr

				err = ld.Run()
				if err != nil {
					logger.Fatal("ERROR: failed to generate executable file with ld. ", err.Error(), ".\n")
				}
			} else {
				outObj, err := os.Create(outPath + ".o")
				if err != nil {
					// TODO if file creating was failed, does Name() method returns valid name??
					logger.Fatalf("ERROR: failed to create file %s. %s.\n", outObj.Name(), err.Error())
				}
				defer func() {
					if err := outObj.Close(); err != nil {
						logger.Fatalf("ERROR: failed to close file %s. %s.\n", outObj.Name(), err.Error())
					}
				}()

				tmpObj, err := os.Open(tmpObjPath)
				if err != nil {
					logger.Fatalf("ERROR: failed to open %s. %s.\n", tmpObjPath, err.Error())
				}
				defer func() {
					if err := tmpObj.Close(); err != nil {
						logger.Fatalf("ERROR: failed to close file %s. %s.\n", outObj.Name(), err.Error())
					}
				}()

				_, err = io.Copy(outObj, tmpObj)
				if err != nil {
					logger.Fatalf("ERROR: failed to %s contents to %s.\n", tmpObjPath, outObj.Name())
				}
				err = outObj.Sync()
				if err != nil {
					logger.Fatalf("ERROR: failed to sync %s. %s.\n", outObj.Name(), err.Error())
				}
			}
		} else {
			// TODO not working creating empty file
			outAsm, err := os.Create(outPath + ".s")
			if verbose {
				logger.Printf("Creating '%s'.\n", outAsm.Name())
			}

			if err != nil {
				// TODO if file creating was failed, does Name() method returns valid name??
				logger.Fatalf("ERROR: failed to create file %s. %s.\n", outAsm.Name(), err.Error())
			}
			defer func() {
				if verbose {
					logger.Printf("Closing '%s'.\n", outAsm.Name())
				}
				if err := outAsm.Close(); err != nil {
					logger.Fatalf("ERROR: failed to close file '%s'. %s.\n", outAsm.Name(), err.Error())
				}
			}()

			if verbose {
				logger.Printf("Syncing '%s'.\n", tmpAsm.Name())
			}
			err = tmpAsm.Sync()
			if err != nil {
				logger.Fatalf("ERROR: failed to sync '%s'. %s.\n", tmpAsm.Name(), err.Error())
			}

			if verbose {
				logger.Printf("Copying '%s' to '%s'.\n", tmpAsm.Name(), outAsm.Name())
			}
			_, err = io.Copy(outAsm, tmpAsm)
			if err != nil {
				logger.Fatalf("ERROR: failed to copy '%s' contents to '%s'.\n", tmpAsm.Name(), outAsm.Name())
			}

			if verbose {
				logger.Printf("Syncing '%s'.\n", outAsm.Name())
			}
			err = outAsm.Sync()
			if err != nil {
				logger.Fatalf("ERROR: failed to sync '%s'. '%s'.\n", outAsm.Name(), err.Error())
			}
		}
	}
}

// TODO move file operations in func
// TODO maybe file.Sync() after all writings
func write(payload string, file *os.File) {
	if _, err := file.WriteString(payload); err != nil {
		logger.Fatal("ERROR: Unable to write to the file ", file.Name(), ". ", err.Error(), "\n")
	}
}
