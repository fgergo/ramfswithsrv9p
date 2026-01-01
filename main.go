// Ramfs serves a memory-based file system.
// It is a demo of srv9p with file trees.
// fgergo: original 9fans.net/go/plan9/srv9p/example/ramfs/main.go
// using github.com/fgergo/p9plib to run on linux
// ; ramfswithsrv9p&
// ; 9 mount `namespace`/ramfswithsrv9p /n/ramfswithsrv9p
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"sync"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/srv9p"

	"github.com/fgergo/p9plib"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: ramfswithsrv9p [-s srvname]\n")
	flag.PrintDefaults()
	os.Exit(1)
}

var (
	srvname = flag.String("s", "ramfswithsrv9p", "post service at /srv/`name`")
	verbose = flag.Bool("v", false, "print protocol trace on standard error")

	srvconn p9plib.Stdio9pserve
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("ramfswithsrv9p: ")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 0 {
		usage()
	}

	args := []string{}
	if *verbose {
		args = append(args, "-v")
	}
	// srv9p.PostMountServe(*srvname, *mtpt, syscall.MREPL|syscall.MCREATE, args, ramfsServer)

	err := p9plib.Post9pservice(&srvconn, *srvname)
	if err != nil {
		log.Fatalf("Post9pservice(), error: %v", err)
	}

	ramfsServer().Serve(srvconn.Stdout9pserve, srvconn.Stdin9pserve)
}

// A ramFile is the per-File storage, saved in the File's Aux field.
type ramFile struct {
	mu   sync.Mutex
	data []byte
}

func ramfsServer() *srv9p.Server {
	srv := &srv9p.Server{
		Tree: srv9p.NewTree("ram", "ram", plan9.DMDIR|0777, nil),
		Open: func(ctx context.Context, fid *srv9p.Fid, mode uint8) error {
			if mode&plan9.OTRUNC != 0 {
				rf := fid.File().Aux.(*ramFile)
				rf.mu.Lock()
				defer rf.mu.Unlock()

				rf.data = nil
			}
			return nil
		},
		Create: func(ctx context.Context, fid *srv9p.Fid, name string, perm plan9.Perm, mode uint8) (plan9.Qid, error) {
			f, err := fid.File().Create(name, "ram", perm, new(ramFile))
			if err != nil {
				return plan9.Qid{}, err
			}
			fid.SetFile(f)
			return f.Stat.Qid, nil
		},
		Read: func(ctx context.Context, fid *srv9p.Fid, data []byte, offset int64) (int, error) {
			rf := fid.File().Aux.(*ramFile)
			rf.mu.Lock()
			defer rf.mu.Unlock()

			return fid.ReadBytes(data, offset, rf.data)
		},
		Write: func(ctx context.Context, fid *srv9p.Fid, data []byte, offset int64) (int, error) {
			rf := fid.File().Aux.(*ramFile)
			rf.mu.Lock()
			defer rf.mu.Unlock()

			if int64(int(offset)) != offset || int(offset)+len(data) < 0 {
				return 0, srv9p.ErrBadOffset
			}
			end := int(offset) + len(data)
			if len(rf.data) < end {
				rf.data = slices.Grow(rf.data, end-len(rf.data))
				rf.data = rf.data[:end]
			}
			copy(rf.data[offset:], data)
			return len(data), nil
		},
	}
	if *verbose {
		srv.Trace = os.Stderr
	}

	return srv
}
