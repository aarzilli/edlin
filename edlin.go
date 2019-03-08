package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/term/termios"
	"io"
	"os"
	"strings"
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
		fmt.Printf(EndOfInputFileMsg)
	} else {
		if !os.IsNotExist(err) {
			fatal("open", err)
		}
		fh, err := os.Create(TheEditor.Path)
		fatal("create", err)
		fh.Close()
		fmt.Printf("New file\n")
	}

	if _, err := os.Stat(TheEditor.Path + "~"); err == nil {
		err := os.Remove(TheEditor.Path + "~")
		fatal("remove backup", err)
	}

	TheEditor.Current = 1

	for {
		fmt.Printf("*")
		cmdstr := TheEditor.Input()

		r := TheEditor.Exec(cmdstr)

		if r == Quit {
			break
		}
	}
}

type Edlin struct {
	Path    string
	Lines   []string
	Current int
	Stdout  io.Writer
	Dirty   bool

	lastNeedle, lastReplace string
}

type ExecReturn uint8

const (
	Continue ExecReturn = iota
	Quit
)

type MoreFn func(prompt string) (out string, ok bool)

const (
	EntryErrMsg       = "Entry error\n"
	EndOfInputFileMsg = "End of input file\n"
	NotFoundMsg       = "Not found\n"
)

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

	qmark := false
	if cmd == '?' && len(rest) > 0 {
		cmd = rest[0]
		rest = rest[1:]
		qmark = true
	}

	cmd = cmd & ^uint8(0x20)

	if qmark && cmd != 'S' && cmd != 'R' {
		fmt.Fprintf(e.Stdout, EntryErrMsg)
		return Continue
	}

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
		if len(params) != 1 {
			panic(EntryErrMsg)
		}
		fmt.Fprintf(e.Stdout, EndOfInputFileMsg)
	case 'C':
		//TODO: copy
	case 'D':
		e.delete(params)
	case 'E':
		e.end(params)
		return Quit
	case 'I':
		//TODO: insert
	case 'L':
		e.display(params, false)
	case 'M':
		//TODO: move
	case 'P':
		e.display(params, true)
	case 'Q':
		if e.quit() == Quit {
			return Quit
		}
	case 'R':
		e.replace(params, rest, qmark)
	case 'S':
		e.search(params, rest, qmark)
	case 'T':
		//TODO: transfer
	case 'W':
		e.write(params)
	default:
		fmt.Fprintf(e.Stdout, EntryErrMsg)
		return Continue
	}

	return Continue
}

// INPUT ////////////////////////////////////////////////////////////////////////////////////////////

func (e *Edlin) Input() string {
	if e.Stdout == nil {
		e.Stdout = os.Stdout
	}

	rr := e.newRawReader()
	defer rr.Close()

	outbuf := []byte{}

	for {
		buf := rr.Next()
		switch len(buf) {
		case 1:
			switch buf[0] {
			case 0x3: // Ctrl-C
				fmt.Fprintf(e.Stdout, "^C")
				rr.Close()
				os.Exit(1)
			case 0x1a: // Ctrl-Z
				fmt.Fprintf(e.Stdout, "^Z")
				outbuf = append(outbuf, 0x1a)
			case 0x7f: // Backspace
				if len(outbuf) > 0 {
					if outbuf[len(outbuf)-1] == 0x1a {
						fmt.Fprintf(e.Stdout, "\x08 \x08")
					}
					fmt.Fprintf(e.Stdout, "\x08 \x08")
					outbuf = outbuf[:len(outbuf)-1]
				}
			case 0xd: // Return
				return string(outbuf)
			default:
				fmt.Fprintf(e.Stdout, "%s", string(buf))
				outbuf = append(outbuf, buf...)
			}
		default:
			fmt.Fprintf(e.Stdout, "%s", string(buf))
			outbuf = append(outbuf, buf...)
		}
	}
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

		added := true

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
			added = false
		}

		if i >= len(cmdstr) {
			return params, 0, ""
		}

		if cmdstr[i] == ' ' {
			i++
		}
		if cmdstr[i] != ',' {
			return params, cmdstr[i], cmdstr[i+1:]
		}
		i++

		if !added {
			params = append(params, 0)
		}

		if len(params) >= 4 {
			return nil, 0, ""
		}
	}

	return nil, 0, ""
}

// COMMANDS ////////////////////////////////////////////////////////////////////////////////////////////

func (e *Edlin) edit(p0 int) {
	e.Current = p0

	if e.Current > len(e.Lines) || e.Current <= 0 {
		return
	}

	fmt.Printf("%7d:*%s\n", e.Current, e.Lines[e.Current-1])
	fmt.Printf("%7d:*", e.Current)

	rr := e.newRawReader()
	defer rr.Close()

	model := e.Lines[e.Current-1]
	mi := 0
	ins := false

	outbuf := []byte{}
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
		buf := rr.Next()

		switch {
		case len(buf) == 1:
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
					if outbuf[len(outbuf)-1] == 0x1a {
						fmt.Fprintf(e.Stdout, "\x08 \x08")
					}
					fmt.Fprintf(e.Stdout, "\x08 \x08")
					outbuf = outbuf[:len(outbuf)-1]
				}
			case 0xd: // Return
				break editLoop
			default:
				emit(buf)
			}

		case bytes.Equal(buf, escSeqDelete):
			//skip a single character from model
			mi++
		case bytes.Equal(buf, escSeqInsert):
			// toggle insert mode (in insert mode typed character won't cause model characters to be skipped)
			ins = !ins
		case bytes.Equal(buf, escSeqF1) || bytes.Equal(buf, escSeqRight):
			// copy a single character from model
			emitModel()
			mi++
		case bytes.Equal(buf, escSeqF3) || bytes.Equal(buf, escSeqEnd):
			// copy everything from model till the end of the line
			for mi < len(model) {
				emitModel()
				mi++
			}
		case bytes.Equal(buf, escSeqF5) || bytes.Equal(buf, escSeqHome):
			// copy current input to model, display a @ to signify that the model was copied
			fmt.Printf("@")
			outbuf = outbuf[:0]
			model = string(outbuf)
			mi = 0

		case bytes.Equal(buf, escSeqF2):
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

		case bytes.Equal(buf, escSeqF4):
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
			// nothing
		}
	}

	if ok {
		e.Dirty = true
		e.Lines[e.Current-1] = string(outbuf)
	}
}

func (e *Edlin) delete(params []int) {
	// two parameters, zero means e.Current for both
	// deletes the interval specified, moves e.Current to the first line after the interval
	p0, p1 := params2(params)
	if p0 == 0 {
		p0 = e.Current
	}
	if p1 == 0 {
		p1 = e.Current
	}
	if p0 > p1 || p0 < 0 || p0 > len(e.Lines) || p1 > len(e.Lines) {
		panic(EntryErrMsg)
	}

	copy(e.Lines[p0-1:], e.Lines[p1:])
	e.Lines = e.Lines[:p0+len(e.Lines[p1:])-1]
	e.Current = p0
}

func (e *Edlin) end(params []int) {
	if len(params) != 0 {
		panic(EntryErrMsg)
	}
	e.write([]int{len(e.Lines)})
	err := os.Rename(e.Path+"~", e.Path)
	fatal("save", err)
}

func (e *Edlin) display(params []int, setcur bool) {
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

	if setcur {
		e.Current = 0
	}

	for i := 0; i < n && (i+start-1 < len(e.Lines)); i++ {
		last := !(i+1 < n && (i+start < len(e.Lines)))
		if last && setcur {
			e.Current = i + start
		}
		iscur := ' '
		if i+start == e.Current {
			iscur = '*'
		}
		fmt.Fprintf(e.Stdout, "%7d:%c%s\n", i+start, iscur, e.Lines[i+start-1])
	}
}

func (e *Edlin) quit() ExecReturn {
	if !e.Dirty {
		return Quit
	}

	switch e.yesno("Abort edit (Y/N)? ", true) {
	case 'Y':
		os.Remove(e.Path + "~")
		return Quit
	default:
		return Continue
	}

}

func (e *Edlin) replace(params []int, needleAndRepl string, qmark bool) {
	p0, p1 := params2(params)
	if p0 == 0 {
		p0 = e.Current + 1
	}
	if p1 == 0 {
		p1 = len(e.Lines) + 1
	}
	var needle, replace string
	if needleAndRepl == "" {
		needle = e.lastNeedle
		replace = e.lastReplace
	} else {
		ctrlz := strings.Index(needleAndRepl, string(0x1a))
		if ctrlz < 0 {
			needle = needleAndRepl
			replace = ""
		} else {
			needle = needleAndRepl[:ctrlz]
			replace = needleAndRepl[ctrlz+1:]
		}
	}
	e.lastNeedle = needle
	e.lastReplace = replace

	for i := p0; i <= len(e.Lines) && i <= p1; i++ {
		s := e.Lines[i-1]
		z := 0
		for {
			o := strings.Index(s[z:], needle)
			if o < 0 {
				break
			}
			z += o

			iscur := ' '
			if i == e.Current {
				iscur = '*'
			}

			doit := true
			if qmark {
				fmt.Fprintf(e.Stdout, "%7d:%c%s\n", i, iscur, s)
				doit = e.yesno("O.K.? ", false) == 'Y'
			}
			if !doit {
				z += len(needle)
				continue
			}

			s = s[:z] + replace + s[z+len(needle):]
			z += len(replace)
			e.Current = i
			e.Dirty = true

			if !qmark {
				fmt.Fprintf(e.Stdout, "%7d:%c%s\n", i, iscur, s)
			}
		}
		e.Lines[i-1] = s
	}
}

func (e *Edlin) search(params []int, needle string, qmark bool) {
	p0, p1 := params2(params)
	if p0 == 0 {
		p0 = e.Current + 1
	}
	if p1 == 0 {
		p1 = len(e.Lines) + 1
	}
	if needle == "" {
		needle = e.lastNeedle
	}
	e.lastNeedle = needle

	for i := p0; i <= len(e.Lines) && i <= p1; i++ {
		if !strings.Contains(e.Lines[i-1], needle) {
			continue
		}
		iscur := ' '
		if i == e.Current {
			iscur = '*'
		}
		fmt.Fprintf(e.Stdout, "%7d:%c%s\n", i, iscur, e.Lines[i-1])
		if !qmark {
			e.Current = i
			return
		}
		if e.yesno("O.K.? ", false) == 'Y' {
			e.Current = i
			return
		}
	}

	fmt.Fprintf(os.Stderr, NotFoundMsg)
}

func (e *Edlin) write(params []int) {
	var n int
	switch len(params) {
	case 0:
		n = len(e.Lines) / 2
	case 1:
		n = params[0]
		if n > len(e.Lines) {
			n = len(e.Lines)
		}
	default:
		panic(EntryErrMsg)
	}
	fh, err := os.OpenFile(e.Path+"~", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	fatal("write", err)
	for i := 0; i < n; i++ {
		n, err := fh.Write([]byte(e.Lines[i]))
		fatal("write", err)
		if n != len(e.Lines[i]) {
			fmt.Fprintf(e.Stdout, "Short write.\n")
			os.Exit(1)
		}
		fh.Write([]byte{'\n'})
	}
	fatal("write", fh.Close())
	copy(e.Lines, e.Lines[n:])
	e.Lines = e.Lines[:len(e.Lines[n:])]
	e.Current = 1
	e.Dirty = true
}

// SUPPORT ////////////////////////////////////////////////////////////////////////////////////////////

func (e *Edlin) yesno(prompt string, strict bool) byte {
	for {
		fmt.Fprintf(e.Stdout, prompt)

		tocooked := setRaw()
		buf := make([]byte, 1)
		_, err := os.Stdin.Read(buf)
		fmt.Fprintf(e.Stdout, "%c", buf[0])
		tocooked()
		fatal("reading term", err)

		buf[0] = buf[0] & ^uint8(0x20)

		if !strict {
			return buf[0]
		}
		switch buf[0] {
		case 'Y', 'N':
			return buf[0]
		}
	}

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

func setRaw() func() {
	var a syscall.Termios
	if err := termios.Tcgetattr(os.Stdin.Fd(), &a); err == nil {
		oldattr := a
		termios.Cfmakeraw(&a)
		termios.Tcsetattr(os.Stdin.Fd(), termios.TCSANOW, &a)
		return func() {
			termios.Tcsetattr(os.Stdin.Fd(), termios.TCSANOW, &oldattr)
			fmt.Printf("\n")
		}
	}
	return func() {}
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

type rawReader struct {
	e        *Edlin
	buf      []byte
	escbuf   []byte
	tocooked func()
}

func (e *Edlin) newRawReader() *rawReader {
	tocooked := setRaw()
	return &rawReader{e, make([]byte, 1), make([]byte, 0, 10), tocooked}
}

func (rr *rawReader) Next() []byte {
	for {
		_, err := os.Stdin.Read(rr.buf)
		fatal("reading term", err)

		switch len(rr.escbuf) {
		case 0:
			if rr.buf[0] == 0x1b { // ESC
				rr.escbuf = append(rr.escbuf, 0x1b)
			} else {
				return rr.buf
			}

		case 1:
			rr.escbuf = append(rr.escbuf, rr.buf[0])
			if rr.buf[0] != '[' {
				// not a CSI
				b := rr.escbuf
				rr.escbuf = rr.escbuf[:0]
				return b
			}

		default:
			rr.escbuf = append(rr.escbuf, rr.buf[0])
			switch {
			case (rr.buf[0] >= 0x20 && rr.buf[0] <= 0x2f) || (rr.buf[0] >= 0x30 && rr.buf[0] <= 0x3f):
				// parameter or intermediate bytes
			default:
				// malformed sequence, flushing
				fallthrough
			case rr.buf[0] >= 0x40 && rr.buf[0] <= 0x7e:
				// final character
				b := rr.escbuf
				rr.escbuf = rr.escbuf[:0]
				return b
			}
		}
	}
}

func (rr *rawReader) Close() {
	rr.tocooked()
}
