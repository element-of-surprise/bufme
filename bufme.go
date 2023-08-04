package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	stdfs "io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"unsafe"

	gfs "github.com/gopherfs/fs"
	"github.com/gopherfs/fs/io/mem/simple"
	osfs "github.com/gopherfs/fs/io/os"
)

const (
	// bufWorkFileDir is a directory in our tmpDir that we store all the full paths to the proto.
	// All the buf YAML files at at the same heirarchy with this directory.
	bufWorkFileDir = "work"
	// genDir is where we generate the pb.go files. This is set by the bufGenFile(which becomes buf.gen.yaml).
	// We move the files out of this into the locations with the .proto files.
	genDir = "generated"
)

var debug = flag.Bool("debug", false, "Turns on debugging")

var (
	bufWorkFile = bytes.TrimSpace([]byte(`
version: v1
directories:
  - work
`))

	bufGenFileTmplText = `
version: v1
plugins:
  - name: go
    out: ./generated/
    opt: paths=source_relative
  - plugin: buf.build/grpc/go:v1.3.0
    out: ./
    opt:
      - paths=source_relative
  - plugin: go-vtproto
    out: ./
    opt:
      - paths=source_relative
{{- range .VTProtoOpts.Pools }}
	  - pool = {{ . }}
{{ end }}
`

	bufGenFileTmpl = template.Must(template.New("bufGenFile").Parse(bufGenFileTmplText))

	bufYAMLFile = bytes.TrimSpace([]byte(`
version: v1
`))
)

// repos is a list of repositories under our config.Root.
var repos []string

var config file

// init reads our configuration file and then reads in our repos under root.
func init() {
	var err error
	config, err = findConfig()
	if err != nil {
		panic(err)
	}

	entries, err := os.ReadDir(config.Root)
	if err != nil {
		panic(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			repos = append(repos, entry.Name())
		}
	}
}

func main() {
	flag.Parse()

	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	inRoot := false
	if strings.HasPrefix(path, config.Root) {
		inRoot = true
	}
	if !inRoot {
		log.Fatalf("you are not currently in your root directory(%s)", config.Root)
	}

	entries, err := os.ReadDir("./")
	if err != nil {
		panic(err)
	}

	protoFiles := []string{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".proto") {
			protoFiles = append(protoFiles, entry.Name())
		}
	}
	if len(protoFiles) != 1 {
		panic("you must have exactly 1 .proto file")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sp := strings.Split(filepath.Join(path, protoFiles[0]), config.Root)
	t := newTree(sp[1])
	if err := t.walk(ctx); err != nil {
		panic(err)
	}

	dir, err := builder(t.fs)
	if err != nil {
		panic(err)
	}
	if *debug {
		log.Println("tmp dir: ", dir)
	} else {
		defer os.RemoveAll(dir)
	}

	if err := runBuf(dir); err != nil {
		panic(err)
	}

	ofs, err := osfs.New()
	if err != nil {
		panic(err)
	}
	tmp, err := ofs.Sub(filepath.Join(dir, genDir))
	if err != nil {
		panic(err)
	}

	err = stdfs.WalkDir(
		tmp,
		".",
		func(path string, d stdfs.DirEntry, err error) error {
			if d.IsDir() {
				return nil
			}
			b, err := ofs.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.HasPrefix(path, genDir+"/") {
				path = strings.Split(path, genDir+"/")[1]
			}
			if strings.HasSuffix(path, "pb.go") {
				if err := os.WriteFile(filepath.Join(config.Root, path), b, 0660); err != nil {
					return err
				}
				fmt.Println("wrote: ", filepath.Join(config.Root, path))
			}
			return nil
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println("Completed!")
}

type tree struct {
	root string

	mu   sync.Mutex
	seen map[string]bool

	fs *simple.FS
	wg sync.WaitGroup
}

func newTree(root string) *tree {
	return &tree{
		root: root,
		seen: map[string]bool{},
		fs:   simple.New(),
	}
}

// walk walks the tree of protos starting at the root file.
func (t *tree) walk(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)

	t.walkNode(ctx, t.root, errCh)
	t.wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
	}
	return nil
}

// walkNode does a concurrent tree decent from file at "p" using its imports as the next files.
// errCh will recieve the first error found. The returned err is only an error found by walking this node
// and can be effectively ignored in lieu of errCh.
func (t *tree) walkNode(ctx context.Context, p string, errCh chan error) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	p = filepath.Join(config.Root, p)
	b, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("cannot open .proto file(%s): %w", p, err)
	}

	p = strings.Split(p, config.Root)[1]
	if err := t.fs.WriteFile(filepath.Join(bufWorkFileDir, p), b, 0600); err != nil {
		return err
	}

	imports, err := getImports(p, b)
	if err != nil {
		return err
	}

	for _, i := range imports {
		i := i

		if ctx.Err() != nil {
			break
		}

		if t.wasWalked(i) {
			continue
		}

		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.walkNode(ctx, i, errCh)
		}()
	}
	return nil
}

// wasWalked determines if this path has been seen before.
func (t *tree) wasWalked(p string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.seen[p] {
		return true
	}
	t.seen[p] = true
	return false
}

// getImports pulls out all imports that are in the proto file and in our root. This assume single line imports.
func getImports(p string, content []byte) ([]string, error) {
	ret := []string{}

	lines := bytes.SplitN(content, []byte("\n"), -1)
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("import")) {
			sp := bytes.Split(line, []byte("import"))
			if len(sp) < 2 {
				return nil, fmt.Errorf("%s: has import line with nothing imported", p)
			}
			line = sp[1]
			if !bytes.HasSuffix(line, []byte(";")) {
				return nil, fmt.Errorf("%s: has import line that doesn't end with ';", p)
			}
			line = bytes.TrimSuffix(line, []byte(";"))
			line = bytes.TrimSpace(line)
			line = bytes.Trim(line, `"`)

			s := byteSlice2String(line)
			for _, r := range repos {
				if strings.HasPrefix(s, r) {
					ret = append(ret, s)
				}
			}
		}
	}
	return ret, nil
}

func builder(fs *simple.FS) (string, error) {
	roots := []string{}
	dirs, err := stdfs.ReadDir(fs, bufWorkFileDir)
	if err != nil {
		return "", err
	}

	// Note: I think this is here for diagnostics.  It's not clear to me any longer
	// that it's needed.
	for _, e := range dirs {
		if e.IsDir() {
			roots = append(roots, e.Name())
		}
	}

	if err := fs.WriteFile("buf.work.yaml", bufWorkFile, 0600); err != nil {
		return "", err
	}
	b := &bytes.Buffer{}
	if err := bufGenFileTmpl.Execute(b, config); err != nil {
		return "", err
	}
	if err := fs.WriteFile("buf.gen.yaml", b.Bytes(), 0600); err != nil {
		return "", err
	}
	if err := fs.WriteFile("buf.yaml", bufYAMLFile, 0600); err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "bufme-*")
	if err != nil {
		return "", err
	}
	o, err := osfs.New()
	if err != nil {
		return "", err
	}
	dst, err := o.Sub(dir)
	if err != nil {
		return "", err
	}

	if err := gfs.Merge(dst.(*osfs.FS), fs, ""); err != nil {
		return "", err
	}

	return dir, nil
}

// runBuf runs the buf command line tool.
func runBuf(dir string) error {
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("could not change directories(%s): %w", dir, err)
	}
	b, err := exec.Command(`buf`, `generate`).CombinedOutput()
	if err != nil {
		return fmt.Errorf("problem running `buf generate`:\n%s", string(b))
	}
	return nil
}

func byteSlice2String(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}
