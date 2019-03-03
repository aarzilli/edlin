package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/term/termios"
	"io"
	"os"
	"syscall"
)

var TheEditor Edlin

func fatal(ctxt string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", ctxt, err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "File name must be specified\n")
		os.Exit(1)
	}

	TheEditor.Path = os.Args[1]

	if fh, err := os.Open(TheEditor.Path); err == nil {
		rd := bufio.NewScanner(fh)
		for rd.Scan() {
			TheEditor.Lines = append(TheEditor.Lines, rd.Text())
		}
		fatal("read", rd.Err())
		fmt.Printf("End of input file\n")
	} else {
		if !os.IsNotExist(err) {
			fatal("open", err)
		}
		fh, err := os.Create(TheEditor.Path)
		fatal("create", err)
		fh.Close()
		fmt.Printf("New file\n")
	}

	TheEditor.Current = 1

	stdin := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("*")
		if !stdin.Scan() {
			break
		}
		cmdstr := stdin.Text()

		r := TheEditor.Exec(cmdstr)

		if r == Quit {
			break
		}
	}
	fatal("reading input", stdin.Err())
}

type Edlin struct {
	Path    string
	Lines   []string
	Current int
	Stdout  io.Writer
	Dirty   bool
}

type ExecReturn uint8

const (
	Continue ExecReturn = iota
	Quit
)

type MoreFn func(prompt string) (out string, ok bool)

const EntryErrMsg = "Entry error\n"

func (e *Edlin) Exec(cmdstr string) ExecReturn {
	defer func() {
		ierr := recover()
		if ierr == nil {
			return
		}
		if errstr, ok := ierr.(string); ok {
			fmt.Fprintf(e.Stdout, errstr)
			return
		}
		panic(ierr)
	}()

	if e.Stdout == nil {
		e.Stdout = os.Stdout
	}
	params, cmd, rest := TheEditor.parse(cmdstr)

	//TODO: sequence of commands separated by semicolon

	if cmd >= 'a' && cmd <= 'z' {
		cmd = cmd - 0x20
	}

	_ = rest

	switch cmd {
	case 0:
		if len(params) == 0 || (len(params) == 1 && params[0]-1 >= len(e.Lines)) {
			return Continue
		}
		if len(params) != 1 {
			fmt.Fprintf(e.Stdout, EntryErrMsg)
			return Continue
		}
		e.edit(params[0])
	case '?':
		//TODO: print help (document function keys for edit?)
	case 'A':
		//TODO: append
	case 'C':
		//TODO: copy
	case 'D':
		//TODO: delete
	case 'E':
		//TODO: save file and exit
	case 'I':
		//TODO: insert
	case 'L':
		e.list(params)
	case 'M':
		//TODO: move
	case 'P':
		//TODO: page
	case 'Q':
		if e.quit() == Quit {
			return Quit
		}
	case 'R':
		//TODO: replace
	case 'S':
		//TODO: search
	case 'T':
		//TODO: transfer
	case 'W':
		//TODO: write
	default:
		fmt.Fprintf(e.Stdout, EntryErrMsg)
		return Continue
	}

	return Continue
}

func (e *Edlin) parse(cmdstr string) (params []int, cmd byte, rest string) {
	// Syntax:
	// cmd ::= <params> <cmdbyte>
	// params ::= <param> | <param> [' '] ',' <params>
	// param ::= '.' | '#' | '+' <number> | '-' <number>

	params = make([]int, 0, 4)

	for i := 0; i < len(cmdstr); {
		readnum := func() int {
			n := 0
			for i < len(cmdstr) && cmdstr[i] >= '0' && cmdstr[i] <= '9' {
				n *= 10
				n += int(cmdstr[i] - '0')
				i++
			}
			return n
		}

		switch cmdstr[i] {
		case '.':
			i++
			params = append(params, e.Current)
		case '#':
			i++
			params = append(params, len(e.Lines)+1)
		case '+':
			i++
			n := e.Current + readnum()
			params = append(params, n)
		case '-':
			i++
			n := e.Current - readnum()
			if n <= 0 {
				n = 1
			}
			params = append(params, n)
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			n := readnum()
			if n <= 0 {
				return nil, 0, ""
			}
			params = append(params, n)
		default:
			params = append(params, 0)
		}

		if i >= len(cmdstr) {
			return params, 0, ""
		}

		if cmdstr[i] == ' ' {
			i++
		}
		if cmdstr[i] != ',' {
			return params, cmdstr[i], cmdstr[i:]
		}
		i++

		if len(params) >= 4 {
			return nil, 0, ""
		}
	}

	return nil, 0, ""
}

var (
	escSeqDelete = []byte{0x1b, 0x5b, 0x33, 0x7e}
	escSeqInsert = []byte{0x1b, 0x5b, 0x32, 0x7e}
	escSeqHome   = []byte{0x1b, 0x5b, 0x37, 0x7e}
	escSeqEnd    = []byte{0x1b, 0x5b, 0x38, 0x7e}
	escSeqF1     = []byte{0x1b, 0x5b, 0x31, 0x31, 0x7e}
	escSeqF2     = []byte{0x1b, 0x5b, 0x31, 0x32, 0x7e}
	escSeqF3     = []byte{0x1b, 0x5b, 0x31, 0x33, 0x7e}
	escSeqF4     = []byte{0x1b, 0x5b, 0x31, 0x34, 0x7e}
	escSeqF5     = []byte{0x1b, 0x5b, 0x31, 0x35, 0x7e}
	escSeqLeft   = []byte{0x1b, 0x5b, 0x44}
	escSeqRight  = []byte{0x1b, 0x5b, 0x43}
	escSeqUp     = []byte{0x1b, 0x5b, 0x41}
	escSeqDown   = []byte{0x1b, 0x5b, 0x42}
)

func (e *Edlin) edit(p0 int) {
	e.Current = p0

	fmt.Printf("%7d:*%s\n", e.Current, e.Lines[e.Current-1])
	fmt.Printf("%7d:*", e.Current)

	var a syscall.Termios
	if err := termios.Tcgetattr(os.Stdin.Fd(), &a); err == nil {
		oldattr := a
		termios.Cfmakeraw(&a)
		termios.Tcsetattr(os.Stdin.Fd(), termios.TCSANOW, &a)
		defer func() {
			termios.Tcsetattr(os.Stdin.Fd(), termios.TCSANOW, &oldattr)
			fmt.Printf("\n")
		}()
	}

	model := e.Lines[e.Current-1]
	mi := 0
	ins := false

	buf := make([]byte, 1)
	outbuf := []byte{}
	escbuf := make([]byte, 0, 10)
	ok := true

	emit := func(buf []byte) {
		fmt.Printf("%s", string(buf))
		outbuf = append(outbuf, buf...)
		if !ins {
			mi += len(buf)
		}
	}

	emitModel := func() {
		if mi >= len(model) {
			return
		}
		fmt.Printf("%c", model[mi])
		outbuf = append(outbuf, model[mi])
	}

editLoop:
	for {
		_, err := os.Stdin.Read(buf)
		fatal("reading term", err)

		switch len(escbuf) {
		case 0:
			switch buf[0] {
			case 0x3: // Ctrl-C
				ok = false
				fmt.Printf("^C")
				break editLoop
			case 0x1a: // Ctrl-Z
				ok = false
				fmt.Printf("^Z")
			case 0x7f: // Backspace
				if len(outbuf) > 0 {
					outbuf = outbuf[:len(outbuf)-1]
				}
			case 0xd: // Return
				break editLoop
			case 0x1b: // ESC
				escbuf = append(escbuf, 0x1b)
			default:
				emit(buf)
			}

		case 1:
			escbuf = append(escbuf, buf[0])
			if buf[0] != '[' {
				// not a CSI
				emit(escbuf)
				escbuf = escbuf[:0]
			}

		default:
			escbuf = append(escbuf, buf[0])
			switch {
			default:
				// malformed sequence, flushing
				fmt.Printf("%s", string(escbuf))
				escbuf = escbuf[:0]
			case (buf[0] >= 0x20 && buf[0] <= 0x2f) || (buf[0] >= 0x30 && buf[0] <= 0x3f):
				// parameter or intermediate bytes
			case buf[0] >= 0x40 && buf[0] <= 0x7e:
				// final character
				switch {
				case bytes.Equal(escbuf, escSeqDelete):
					//skip a single character from model
					mi++
				case bytes.Equal(escbuf, escSeqInsert):
					// toggle insert mode (in insert mode typed character won't cause model characters to be skipped)
					ins = !ins
				case bytes.Equal(escbuf, escSeqF1) || bytes.Equal(escbuf, escSeqRight):
					// copy a single character from model
					emitModel()
					mi++
				case bytes.Equal(escbuf, escSeqF3) || bytes.Equal(escbuf, escSeqEnd):
					// copy everything from model till the end of the line
					for mi < len(model) {
						emitModel()
						mi++
					}
				case bytes.Equal(escbuf, escSeqF5) || bytes.Equal(escbuf, escSeqHome):
					// copy current input to model, display a @ to signify that the model was copied
					fmt.Printf("@")
					outbuf = outbuf[:0]
					model = string(outbuf)
					mi = 0

				case bytes.Equal(escbuf, escSeqF2):
					// copy everythin from model till the first match of the argument character
					_, err := os.Stdin.Read(buf)
					fatal("reading term", err)
					for mi < len(model) {
						if model[mi] == buf[0] {
							break
						}
						emitModel()
						mi++
					}

				case bytes.Equal(escbuf, escSeqF4):
					//skip everything on model till the first match of the argument character
					_, err := os.Stdin.Read(buf)
					fatal("reading term", err)
					for mi < len(model) {
						if model[mi] == buf[0] {
							break
						}
						mi++
					}

				default:
					fmt.Printf("%x", escbuf)
				}
				escbuf = escbuf[:0]
			}
		}
	}

	if ok {
		e.Dirty = true
		e.Lines[e.Current-1] = string(outbuf)
	}
}

func (e *Edlin) list(params []int) {
	p0, p1 := params2(params)

	start := e.Current - 11
	if start <= 0 {
		start = 1
	}
	n := 23

	if p0 != 0 {
		start = p0
	}
	if p1 != 0 {
		n = p1 - start + 1
	}

	if n <= 0 {
		n = 23
	}

	for i := 0; i < n && (i+start-1 < len(e.Lines)); i++ {
		iscur := ' '
		if i+start == e.Current {
			iscur = '*'
		}
		fmt.Fprintf(e.Stdout, "%7d:%c%s\n", i+start, iscur, e.Lines[i+start-1])
	}
}

func (e *Edlin) quit() ExecReturn {
	//TODO: if dirty ask confirmation
	return Quit
}

func params2(params []int) (int, int) {
	var p0, p1 int
	switch len(params) {
	case 0:
		// use defaults
	case 2:
		p1 = params[1]
		fallthrough
	case 1:
		p0 = params[0]
	default:
		panic(EntryErrMsg)
	}
	if p1 != 0 && p0 > p1 {
		panic(EntryErrMsg)
	}
	return p0, p1
}
