package main

import (
	"fmt"
	"os"

	"github.com/aquaflamingo/stem/internal/als"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "rebuild":
		err = cmdRebuild(args)
	case "info":
		err = cmdInfo(args)
	case "check":
		err = cmdCheck(args)
	case "decompress":
		err = cmdDecompress(args)
	case "explode":
		err = cmdExplode(args)
	case "implode":
		err = cmdImplode(args)
	case "diff":
		err = cmdDiff(args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "stem: unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "stem: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `stem — Live Set version control (prototype)

usage: stem <command> [args]

  init [dir]                adopt a project folder as a Stem repo (default .)
  rebuild [dir]             regenerate the working .als from set/
  info <set.als>            print version, entity and track summary
  check <set.als>           parse and verify a byte-exact round trip
  decompress <in.als> <out.xml>
  explode <in.als> <dir>    canonical tree (SID-addressed) -> directory
  implode <dir> <out.als>   canonical directory -> .als
  diff <a.als> <b.als>      semantic diff between two Sets
`)
}

func requireArgs(args []string, n int, form string) []string {
	if len(args) < n {
		fmt.Fprintf(os.Stderr, "stem: expected %s\n", form)
		os.Exit(2)
	}
	return args
}

func loadDoc(path string) (*als.Doc, error) {
	d, err := als.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return d, nil
}
