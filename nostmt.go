package main

import (
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"io"
	"os"
	"sort"
)

func open(path string) (*dwarf.Data, error) {
	if fh, err := elf.Open(path); err == nil {
		return fh.DWARF()
	}

	if fh, err := pe.Open(path); err == nil {
		return fh.DWARF()
	}

	if fh, err := macho.Open(path); err == nil {
		return fh.DWARF()
	}

	return nil, fmt.Errorf("unrecognized executable format")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type Line struct {
	File string
	Line int
}

func main() {
	lines := map[Line]bool{}
	dw, err := open(os.Args[1])
	must(err)
	rdr := dw.Reader()
	rdr.Seek(0)
	for {
		e, err := rdr.Next()
		must(err)
		if e == nil {
			break
		}
		if e.Tag != dwarf.TagCompileUnit {
			continue
		}
		lrdr, err := dw.LineReader(e)
		must(err)

		var le dwarf.LineEntry

		for {
			err := lrdr.Next(&le)
			if err == io.EOF {
				break
			}
			must(err)
			fl := Line{le.File.Name, le.Line}
			lines[fl] = lines[fl] || le.IsStmt
		}
	}

	nonStmtLines := []Line{}
	for line, isstmt := range lines {
		if !isstmt {
			nonStmtLines = append(nonStmtLines, line)
		}
	}
	sort.Slice(nonStmtLines, func(i, j int) bool {
		if nonStmtLines[i].File == nonStmtLines[j].File {
			return nonStmtLines[i].Line < nonStmtLines[j].Line
		}
		return nonStmtLines[i].File < nonStmtLines[j].File
	})
	for _, line := range nonStmtLines {
		fmt.Printf("%s:%d\n", line.File, line.Line)
	}
}
