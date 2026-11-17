package objects

import (
	"context"
	"strings"
	"sync"
	"time"
)

type Fake struct {
	mu   sync.Mutex
	objs map[string]Info
	err  error
}

func NewFake() *Fake { return &Fake{objs: map[string]Info{}} }

func (f *Fake) Put(key, contentType string, size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objs[key] = Info{Size: size, ContentType: contentType}
}

func (f *Fake) Has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objs[key]
	return ok
}

func (f *Fake) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *Fake) PresignPut(_ context.Context, key, _ string, _ int64, _ time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	return "http://fake-storage/put/" + key, nil
}

func (f *Fake) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	return "http://fake-storage/get/" + key, nil
}

func (f *Fake) Stat(_ context.Context, key string) (Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return Info{}, f.err
	}
	info, ok := f.objs[key]
	if !ok {
		return Info{}, ErrNotFound
	}
	return info, nil
}

func (f *Fake) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	delete(f.objs, key)
	return nil
}

func (f *Fake) DeletePrefix(_ context.Context, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	for k := range f.objs {
		if strings.HasPrefix(k, prefix) {
			delete(f.objs, k)
		}
	}
	return nil
}
