package main

import (
	"bufio"
	"fmt"
	_ "io/ioutil"
	"os"
)

var Path string
var Lines []string

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

	Path = os.Args[1]

	if fh, err := os.Open(Path); err == nil {
		rd := bufio.NewScanner(fh)
		for rd.Scan() {
			Lines = append(Lines, rd.Text())
		}
		fatal("read", rd.Err())
	} else {
		if !os.IsNotExist(err) {
			fatal("open", err)
		}
		fh, err := os.Create(Path)
		fatal("create", err)
		fh.Close()
	}

	stdin := bufio.NewScanner(os.Stdin)
	for stdin.Scan() {
		cmdstr := stdin.Text()
		if len(cmdstr) <= 0 {
			continue
		}

	}
	fatal("reading input", stdin.Err())
}
