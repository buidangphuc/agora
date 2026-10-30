package app

import (
	"io/fs"
	"testing/fstest"
)

type tempFS map[string][]byte

func (m tempFS) fs() fs.FS {
	out := fstest.MapFS{}
	for k, v := range m {
		out[k] = &fstest.MapFile{Data: v}
	}
	return out
}
