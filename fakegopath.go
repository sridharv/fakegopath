// Package fakegopath provides utilities to create temporary go source trees.
package fakegopath

import (
	"bytes"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"text/template"
)

// Temporary is a temporary go source tree. The path is optionally appended to go.build.Default.GOPATH.
type Temporary struct {
	Path      string // The path that is appended.
	Orig      string // The original GOPATH
	Pkg       string // The pkg directory
	Src       string // The src directory
	Bin       string // The bin directory
	update    bool
	deleteDir bool
}

type SourceFile struct {
	Src     string
	Dest    string
	Content []byte
}

// NewTemporaryWithFiles creates a temporary go source tree after copying/creating files.
// prefix is used to create a temporary directory in which the source tree is created.
func NewTemporaryWithFiles(prefix string, files []SourceFile) (*Temporary, error) {
	dir, err := ioutil.TempDir("", prefix)
	if err != nil {
		return nil, err
	}
	t, err := NewTemporary(dir, true)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	t.deleteDir = true
	if err := t.Copy(files); err != nil {
		t.Reset()
		return nil, err
	}
	return t, nil
}

// CopyFiles copies all source files in files using t.CopyFile or t.WriteFile as needed.
func (t *Temporary) Copy(files []SourceFile) error {
	for _, f := range files {
		if f.Content != nil {
			if err := t.WriteFile(f.Dest, bytes.NewBuffer(f.Content)); err != nil {
				return err
			}
			continue
		}
		if err := t.CopyFile(f.Dest, f.Src); err != nil {
			return err
		}
	}
	return nil
}

func (t *Temporary) KeepTempDir(keep bool) { t.deleteDir = !keep }

// NewTemporary creates a temporary under the specified directory.
// If updateGoPath is true, go.build.Default.GOPATH will have this path prefixed to it.
func NewTemporary(dir string, updateGoPath bool) (*Temporary, error) {
	t := &Temporary{
		Path:      dir,
		Pkg:       filepath.Join(dir, "pkg"),
		Src:       filepath.Join(dir, "src"),
		Bin:       filepath.Join(dir, "bin"),
		update:    updateGoPath,
		deleteDir: false,
	}

	for _, d := range []string{t.Src, t.Pkg, t.Bin} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return nil, fmt.Errorf("failed to create %s: %v", d, err)
		}
	}

	if t.update {
		t.Orig = build.Default.GOPATH
		if os.Getenv("GOPATH") != t.Orig {
			return nil, fmt.Errorf("GOPATH %s doesn't match build.Default.GOPATH %s", os.Getenv("GOPATH"), t.Orig)
		}
		build.Default.GOPATH = dir + ":" + build.Default.GOPATH
		os.Setenv("GOPATH", build.Default.GOPATH)
	}
	return t, nil
}

func loggedClose(file string, closer io.Closer) { logError("failed to close "+file, closer.Close()) }
func logError(msg string, err error) {
	if err != nil {
		log.Println(msg, err)
	}
}

// GenerateFile is equivalent to calling WriteFile with the results of tpl.Execute(..., args)
func (t *Temporary) GenerateFile(file string, tpl *template.Template, args interface{}) error {
	buf := bytes.NewBuffer([]byte{})
	if err := tpl.Execute(buf, args); err != nil {
		return fmt.Errorf("failed to generate: %v", err)
	}
	return t.WriteFile(file, buf)
}

// CopyFile is equivalent to WriteFile with the contents of src.
func (t *Temporary) CopyFile(dest, src string) error {
	input, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %v", src, err)
	}
	defer loggedClose(src, input)
	return t.WriteFile(dest, input)
}

// WriteFile writes contents to file, where file is a path relative to the src directory.
// Any intermediate directories are created if needed.
func (t *Temporary) WriteFile(file string, contents io.Reader) error {
	fullPath := filepath.Join(t.Src, file)
	fileDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(fileDir, 0700); err != nil {
		return fmt.Errorf("failed to create dir %s: %v", fileDir, err)
	}
	w, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("couldn't open %s for writing: %v", fullPath, err)
	}
	defer loggedClose(fullPath, w)
	if _, err := io.Copy(w, contents); err != nil {
		return fmt.Errorf("copy failed: %v", err)
	}
	return nil
}

// Reset resets the original GOPATH and deletes the temporary directory.
func (t *Temporary) Reset() {
	if t.update {
		build.Default.GOPATH = t.Orig
		os.Setenv("GOPATH", t.Orig)
	}
	if t.deleteDir {
		if err := os.RemoveAll(t.Path); err != nil {
			log.Println(err)
		}
	}
}
