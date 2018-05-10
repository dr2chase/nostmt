package main

import (
	"bufio"
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"
)

var noshowline = flag.Bool("q", false, "does not show line contents")
var showruntime = flag.Bool("r", false, "shows hits in runtime package")
var bekind = flag.Bool("k", false, "suppress some false positives")
var countonly = flag.Bool("c", false, "only show counts of total and missed")

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

func display(line Line) int {
	var ok bool
	var t string
	var file *File
	if !*noshowline || *bekind {
		file = loadFile(line.File)
		t, ok = file.Get(line.Line)
	}
	if *bekind && suppress(file, line.Line) {
		return 0
	}
	if *countonly {
		return 1
	}
	if !ok || *noshowline {
		fmt.Printf("%s:%d\n", line.File, line.Line)
		return 1
	}
	fmt.Printf("%s:%d: %s\n", line.File, line.Line, t)
	return 1
}

func suppress(file *File, lineno int) bool {
	// this assumes go formatted code

	line, _ := file.Get(lineno)
	line = strings.TrimSpace(line)

	// suppress empty lines, just so I don't have to worry about empty lines in the following
	if line == "" {
		return true
	}

	// suppress lines that have nothing but symbols in them
	onlysyms := true
	for _, ch := range line {
		if unicode.IsLetter(ch) || unicode.IsNumber(ch) {
			onlysyms = false
			break
		}
	}
	if onlysyms {
		return true
	}

	// suppress function headings
	if strings.HasPrefix(line, "func ") {
		return true
	}

	// suppress clauseless for and switch headings
	if line == "for {" || line == "switch {" {
		return true
	}

	// suppress switch clauses
	if line == "default:" || (strings.HasPrefix(line, "case ") && line[len(line)-1] == ':') {
		return true
	}

	// suppress variable declarations without initialization
	if strings.HasPrefix(line, "var ") && !strings.Contains(line, "=") {
		return true
	}

	return false
}

type File struct {
	lines []string
}

var fileCache = map[string]*File{}

func loadFile(filename string) *File {
	if r := fileCache[filename]; r != nil {
		return r
	}

	fh, err := os.Open(filename)
	if err != nil {
		return nil
	}
	s := bufio.NewScanner(fh)
	r := &File{}
	for s.Scan() {
		t := s.Text()
		if len(t) > 0 && t[len(t)-1] == '\n' {
			t = t[:len(t)-1]
		}
		r.lines = append(r.lines, t)
	}
	if s.Err() != nil {
		return nil
	}
	fileCache[filename] = r
	return r
}

func (f *File) Get(lineno int) (string, bool) {
	if f == nil {
		return "", false
	}
	if lineno-1 < 0 || lineno-1 >= len(f.lines) {
		return "", false
	}
	return f.lines[lineno-1], true
}

func main() {
	flag.Parse()
	lines := map[Line]bool{}
	dw, err := open(flag.Args()[0])
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
		pkgname, _ := e.Val(dwarf.AttrName).(string)
		if pkgname == "runtime" {
			if !*showruntime {
				continue
			}
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
	count := 0
	for _, line := range nonStmtLines {
		count += display(line)
	}
	if *countonly {
		fmt.Printf("total=%d, nostmt=%d\n", len(lines), count)
	}
}
