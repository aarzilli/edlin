package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

const vispaTeresa = `La vispa Teresa
avea tra l'erbetta
A volo sorpresa
gentil farfalletta
E tutta giuliva
stringendola viva
gridava a distesa:
“L'ho presa! L'ho presa!”.
A lei supplicando
l'afflitta gridò:
“Vivendo, volando
che male ti fò?
Tu sì mi fai male
stringendomi l'ale!
Deh, lasciami! Anch'io
son figlia di Dio!”.
Teresa pentita
allenta le dita:
“Va', torna all'erbetta,
gentil farfalletta”.
Confusa, pentita,
Teresa arrossì,
dischiuse le dita
e quella fuggì`

func testCommandIntl(t *testing.T, before, after string, cur int, command string, output string) (*Edlin, string) {
	var e Edlin
	var out bytes.Buffer
	e.Stdout = &out
	e.Current = cur
	if before != "" {
		e.Lines = strings.Split(before, "\n")
	}
	e.Exec(command)
	t.Logf("<%s> -> %s\n", command, out.String())
	if output != "*" && out.String() != output {
		t.Errorf("error executing %q, output mismatch", command)
	}
	oafter := strings.Join(e.Lines, "\n")
	if after != oafter {
		t.Errorf("buffer mismatch after executing %q: %q", command, oafter)
	}
	return &e, out.String()
}

func testCommand(t *testing.T, before, after string, cur int, command string, output string) (e *Edlin, o string) {
	t.Run(command, func(t *testing.T) {
		e, o = testCommandIntl(t, before, after, cur, command, output)
	})
	return e, o
}

func testList(t *testing.T, before, command string, start, end, cur int) {
	listLineno := func(line string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(strings.SplitN(line, ":", 2)[0]))
		return n
	}

	t.Run(command, func(t *testing.T) {
		_, out := testCommandIntl(t, before, before, cur, command, "*")
		v := strings.Split(out, "\n")
		for i := len(v) - 1; i >= 0; i-- {
			if v[i] != "" {
				break
			}
			v = v[:i]
		}
		if n := listLineno(v[0]); n != start {
			t.Errorf("wrong start line number %d", n)
		}
		if n := listLineno(v[len(v)-1]); n != end {
			t.Errorf("wrong end line number %d", n)
		}
		for i := range v {
			if strings.SplitN(v[i], ":", 2)[1][0] == '*' {
				if curlineno := listLineno(v[i]); curlineno != cur {
					t.Errorf("wrong current line number %d", curlineno)
				}
				break
			}
		}
	})
}

func TestList(t *testing.T) {
	testList(t, vispaTeresa, "l", 1, 23, 1)
	testList(t, vispaTeresa, "1,l", 1, 23, 1)
	testCommand(t, vispaTeresa, vispaTeresa, 1, "2,1l", EntryErrMsg)
	testList(t, vispaTeresa, "2l", 2, 24, 1)
	testList(t, vispaTeresa, "2,l", 2, 24, 1)
	testList(t, vispaTeresa, "2,2l", 2, 2, 1)
	testCommand(t, "", "", 1, "l", "")
	testList(t, vispaTeresa, ",5l", 1, 5, 1)
	testList(t, vispaTeresa, ",5l", 1, 5, 3)
	testList(t, vispaTeresa, ",5l", 9, 24, 20)
}

func TestSearch(t *testing.T) {
	testCommandIntl(t, vispaTeresa, vispaTeresa, 1, "ser", "*")
}

func assertCurrent(t *testing.T, e *Edlin, n int) {
	if e.Current != n {
		t.Fatalf("expected current %d got %d\n", n, e.Current)
	}
}

func TestCopy(t *testing.T) {
	e, _ := testCommand(t,
		"uno\ndue\ntre\nquattro\ncinque\nsei\n",
		"cinque\nuno\ndue\ntre\nquattro\ncinque\nsei\n", 1, "5,5,1c", "*")
	assertCurrent(t, e, 1)
	e, _ = testCommand(t,
		"uno\ndue\ntre\nquattro\ncinque\nsei\n",
		"uno\ndue\ntre\nquattro\nuno\ndue\ntre\ncinque\nsei\n", 1, "1,3,5c", "*")
	assertCurrent(t, e, 5)
	e, _ = testCommand(t,
		"uno\ndue\ntre\nquattro\ncinque\nsei\n",
		"cinque\nuno\ndue\ntre\nquattro\nsei\n", 1, "5,5,1m", "*")
	assertCurrent(t, e, 1)
	e, _ = testCommand(t,
		"uno\ndue\ntre\nquattro\ncinque\nsei\n",
		"quattro\nuno\ndue\ntre\ncinque\nsei\n", 1, "1,3,5m", "*")
	assertCurrent(t, e, 2)
	testCommand(t,
		"uno\ndue\ntre\nquattro\ncinque\nsei\n",
		"uno\ncinque\ncinque\ncinque\ndue\ntre\nquattro\ncinque\nsei\n", 1, "5,5,2,3c", "*")
	testCommand(t,
		"uno\ndue\ntre\nquattro\ncinque\nsei\n",
		"uno\ndue\ntre\nquattro\ncinque\nsei\nuno\ndue\n", 1, "1,2,#c", "*")
	testCommand(t,
		"uno\ndue\ntre\nquattro\ncinque\nsei\n",
		"tre\nquattro\ncinque\nsei\nuno\ndue\n", 1, "1,2,#m", "*")
}
