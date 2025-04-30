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
	flag.BoolVar(&genAsm, "S", false, "Generate .s files with GNU assembly translation of provided .bf source code.")
	flag.BoolVar(&genObject, "c", false, "Generate object files.")

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
		logv("Opening '%s'\n", srcPath)
		srcFile, err := os.Open(srcPath)
		if err != nil {
			logger.Fatalf("ERROR: unable to read the file '%s'. %s\n", srcPath, err.Error())
		}

		// generating out file name if not provided, or if source files more than one
		if outPath == "" || len(sourceFiles) > 1 {
			ex, err := os.Executable()
			if err != nil {
				logger.Fatalln("ERROR: unable to get path name for the compiler executable")
			}

			// outPath = current working dir for compiler + name of .bf file without .bf extension
			if !strings.Contains(srcPath, ".bf") {
				logger.Printf("%s: ERROR: this file does not have .bf extension. Ignoring this file. Please provide source code files only with .bf extension\n", srcPath)
				continue
			}

			baseSrc := filepath.Base(srcPath)
			outPath = filepath.Dir(ex) + "/" + baseSrc[:len(baseSrc)-3]
		}

		tmpAsm, err := os.CreateTemp("", "mgbfc-*.s")
		if err != nil {
			logger.Fatalf("ERROR: unable to create the temporary assembly file \n")
		}

		tmpAsmName := tmpAsm.Name()
		logv("Creating '%s'\n", tmpAsmName)

		defer func() {
			logv("Removing '%s'\n", tmpAsmName)
			if err := os.Remove(tmpAsmName); err != nil {
				logger.Printf("WARNING: unable to remove the file '%s'. %s\n", tmpAsmName, err.Error())
			}
		}()

		logv("Started to parse source file '%s' and writing asm to tmp file\n", srcPath)

		// generating prologue
		_, err = tmpAsm.WriteString(fmt.Sprintf(rodata, tapeSize) + bss + text)
		if err != nil {
			logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
		}

		var dataPtr, line, col uint = 0, 1, 0

		labelIdx := -1
		label := -1
		labels := make([]uint, 25)

		var bracketNestinglvl uint = 0
		var firstBracketLine, firstBracketCol uint

		// reading the bf code file and generating the asm
		reader := bufio.NewReader(srcFile)
		for {
			char, _, err := reader.ReadRune()
			if err != nil {
				if err == io.EOF {
					if bracketNestinglvl != 0 {
						logger.Fatalf("%s:%d:%d: ERROR: unclosed bracket", srcPath, firstBracketLine, firstBracketCol)
					}

					_, err := tmpAsm.WriteString(epilogue)
					if err != nil {
						logger.Fatalf("ERROR: unable to write to '%s'. %s\n", tmpAsmName, err.Error())
					}
					break
				} else {
					logger.Fatalf("%s:%d:%d: ERROR: unable to read character from '%s'. %s\n", srcPath, line, col, srcFile.Name(), err.Error())
				}
			}

			// r9 for data pointer
			// r10 for main array pointer
			// r11 as intermediate reg
			switch char {
			// TODO optimize many add/subs/shifts in a row
			case '+':
				_, err := tmpAsm.WriteString(increment)
				if err != nil {
					logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
				}
			case '-':
				_, err := tmpAsm.WriteString(decrement)
				if err != nil {
					logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
				}
			case '>':
				if dataPtr == (tapeSize - 1) {
					dataPtr = 0
					_, err := tmpAsm.WriteString(resetDataPtr)
					if err != nil {
						logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
					}
				} else {
					dataPtr++
					_, err := tmpAsm.WriteString(incrDataPtr)
					if err != nil {
						logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
					}
				}
			case '<':
				if dataPtr == 0 {
					dataPtr = tapeSize - 1
					_, err := tmpAsm.WriteString(maximizeDataPtr)
					if err != nil {
						logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
					}
				} else {
					dataPtr--
					_, err := tmpAsm.WriteString(decrDataPtr)
					if err != nil {
						logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
					}
				}
			case ',':
				_, err := tmpAsm.WriteString(input)
				if err != nil {
					logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
				}
			case '.':
				_, err := tmpAsm.WriteString(output)
				if err != nil {
					logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
				}
			case '[':
				if bracketNestinglvl == 0 {
					firstBracketLine = line
					firstBracketCol = col
				}
				bracketNestinglvl++

				label++
				labelIdx++
				labels[labelIdx] = uint(label)

				_, err := tmpAsm.WriteString(fmt.Sprintf(cycleStart, label, label))
				if err != nil {
					logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
				}
			case ']':
				bracketNestinglvl--

				_, err := tmpAsm.WriteString(fmt.Sprintf(cycleEnd, labels[labelIdx], labels[labelIdx]))
				if err != nil {
					logger.Fatalf("ERROR: unable to write to the file '%s'. %s\n", tmpAsmName, err.Error())
				}
				labelIdx--
			case '#':
				_, err := reader.ReadString('\n')
				if err != nil {
					logger.Fatalf("%d: ERROR: unable to read line from '%s'. %s\n", line, srcFile.Name(), err.Error())
				}
				fallthrough
			case '\n':
				line++
				col = 0
			case ' ':
			default:
				logger.Fatalf("%s:%d:%d: ERROR: unexcepted token: '%c'\n", srcFile.Name(), line, col, char)
			}

			col++
		}

		// TODO if error is occured tmp files will not be removed

		// closing tmp assembly file before generating output file
		logv("Closing '%s'\n", tmpAsmName)
		if err := tmpAsm.Close(); err != nil {
			logger.Printf("WARNING: unable to close the file '%s'. %s\n", tmpAsmName, err.Error())
		}

		logv("Parsing '%s' and generating assembly completed", srcPath)

		if genAsm {
			err := generateOutAsm(outPath+".s", tmpAsmName)
			if err != nil {
				logger.Fatal(err.Error())
			}
		}

		if genObject {
			tmpObjPath, err := generateTmpObj(tmpAsmName)
			// TODO if error is occured here, is it safe to defer removing?
			if err != nil {
				logger.Fatal(err.Error())
			}

			defer func() {
				logv("Removing '%s'\n", tmpObjPath)
				if err := os.Remove(tmpObjPath); err != nil {
					logger.Printf("WARNING: unable to remove the file '%s'. %s\n", tmpObjPath, err.Error())
				}
			}()

			err = generateOutObj(outPath+".o", tmpObjPath)
			if err != nil {
				logger.Fatal(err.Error())
			}
		}

		if !genAsm && !genObject {
			tmpObjPath, err := generateTmpObj(tmpAsmName)
			// TODO if error is occured here, is it safe to defer removing?
			if err != nil {
				logger.Fatal(err.Error())
			}

			defer func() {
				logv("Removing '%s'\n", tmpObjPath)
				if err := os.Remove(tmpObjPath); err != nil {
					logger.Printf("WARNING: unable to remove the file '%s'. %s\n", tmpObjPath, err.Error())
				}
			}()

			err = generateOutExe(outPath, tmpObjPath)
			if err != nil {
				logger.Fatal(err.Error())
			}
		}

		logv("Closing '%s'\n", srcFile.Name())
		if err := srcFile.Close(); err != nil {
			logger.Printf("WARNING: unable to close the file '%s'. %s\n", srcFile.Name(), err.Error())
		}

		logv("Compilation completed\n")
	}
}

func generateOutAsm(outPath string, tmpAsmName string) error {
	logv("Creating final out assembly file\n")

	logv("Creating '%s'\n", outPath)
	outAsm, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("ERROR: failed to create file '%s'. %s\n", outPath, err.Error())
	}

	defer func() {
		logv("Closing '%s'\n", outAsm.Name())
		if err := outAsm.Close(); err != nil {
			logger.Printf("WARNING: unable to close the file '%s'. %s\n", outAsm.Name(), err.Error())
		}
	}()

	logv("Opening '%s'\n", tmpAsmName)
	tmpAsm, err := os.Open(tmpAsmName)
	if err != nil {
		return fmt.Errorf("ERROR: unable to read the file '%s'. %s\n", tmpAsmName, err.Error())
	}

	defer func() {
		logv("Closing '%s'\n", tmpAsmName)
		if err := tmpAsm.Close(); err != nil {
			logger.Printf("WARNING: unable to close the file '%s'. %s\n", tmpAsmName, err.Error())
		}
	}()

	logv("Copying '%s' to '%s'.\n", tmpAsmName, outAsm.Name())
	_, err = io.Copy(outAsm, tmpAsm)
	if err != nil {
		return fmt.Errorf("ERROR: failed to copy '%s' contents to '%s'. %s\n", tmpAsmName, outAsm.Name(), err.Error())
	}

	logv("Syncing '%s'\n", outAsm.Name())
	err = outAsm.Sync()
	if err != nil {
		return fmt.Errorf("ERROR: failed to sync '%s'. %s\n", outAsm.Name(), err.Error())
	}

	return nil
}

func generateTmpObj(tmpAsmName string) (string, error) {
	logv("Generating temporary object file\n")

	tmpFilePath := tmpAsmName[:len(tmpAsmName)-2]
	tmpObjPath := tmpFilePath + ".o"

	// TODO maybe add fasm
	cmdName := "as"
	cmdArgs := []string{tmpAsmName, "-o", tmpObjPath}

	err := executeCommand(cmdName, cmdArgs...)
	if err != nil {
		return "", fmt.Errorf("ERROR: unable to execute '%s %s'. %s\n", cmdName, strings.Join(cmdArgs, " "), err.Error())
	}

	return tmpObjPath, nil
}

func generateOutObj(outPath string, tmpObjPath string) error {
	logv("Generating final out object file\n")

	logv("Creating '%s'\n", outPath)
	outObj, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("ERROR: unable to create file '%s'. %s\n", outPath, err.Error())
	}

	defer func() {
		logv("Closing '%s'\n", outObj.Name())
		if err := outObj.Close(); err != nil {
			logger.Printf("WARNING: unable to close the file '%s'. %s\n", outObj.Name(), err.Error())
		}
	}()

	logv("Opening '%s'\n", tmpObjPath)
	tmpObj, err := os.Open(tmpObjPath)
	if err != nil {
		return fmt.Errorf("ERROR: unable to open the file '%s'. %s\n", tmpObjPath, err.Error())
	}

	defer func() {
		logv("Closing '%s'\n", tmpObj.Name())
		if err := tmpObj.Close(); err != nil {
			logger.Printf("WARNING: unable to close the file '%s'. %s\n", tmpObj.Name(), err.Error())
		}
	}()

	logv("Copying '%s' to '%s'.\n", tmpObj.Name(), outObj.Name())
	_, err = io.Copy(outObj, tmpObj)
	if err != nil {
		return fmt.Errorf("ERROR: failed to copy '%s' contents to '%s'. %s\n", tmpObj.Name(), outObj.Name(), err.Error())
	}

	return nil
}

func generateOutExe(outPath string, tmpObjPath string) error {
	logv("Generating final executable\n")

	// TODO I can generate ELF64 header by myself https://tuket.github.io/notes/asm/elf64_hello_world
	cmdName := "ld"
	cmdArgs := []string{tmpObjPath, "-o", outPath}

	err := executeCommand(cmdName, cmdArgs...)
	if err != nil {
		return fmt.Errorf("ERROR: unable to execute '%s %s'. %s\n", cmdName, strings.Join(cmdArgs, " "), err.Error())
	}

	return nil
}

func logv(format string, args ...any) {
	if verbose {
		logger.Printf(format, args...)
	}
}

func executeCommand(name string, args ...string) error {
	logv("Executing '%s %s'.\n", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
