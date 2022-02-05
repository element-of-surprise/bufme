package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

const (
	name = "bufme.conf"
)

type file struct {
	// Root is the root path that holds all your git repos.
	Root string
}

func (f *file) validate(p string) error {
	if f.Root == "." {
		u, err := user.Current()
		if err != nil {
			return err
		}
		if filepath.Clean(p) == filepath.Clean(u.HomeDir) {
			return fmt.Errorf("cannot use set Root == '.' if the config file is in your home directory")
		}
		f.Root = filepath.Clean(p)
	}
	if _, err := os.Stat(f.Root); err != nil {
		return fmt.Errorf("Root(%s) does not exist in the file system: %s", f.Root, err)
	}
	return nil
}

func fromFile(p string) (file, error) {
	f := file{}

	b, err := os.ReadFile(p)
	if err != nil {
		return file{}, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return file{}, err
	}
	if err := f.validate(p); err != nil {
		return file{}, err
	}
	return f, nil
}

func findConfig() (file, error) {
	d, err := os.Getwd()
	if err != nil {
		return file{}, err
	}

	for {
		p := filepath.Join(d, name)
		_, err := os.Stat(p)
		if err == nil {
			return fromFile(p)
		}

		d = filepath.Dir(d)
		if d == "/" {
			break
		}
	}

	u, err := user.Current()
	if err != nil {
		return file{}, err
	}

	if _, err := os.Stat(filepath.Join(u.HomeDir, name)); err == nil {
		return fromFile(filepath.Join(u.HomeDir, name))
	}

	return file{}, fmt.Errorf("didn't find a %s anywhere", name)
}
