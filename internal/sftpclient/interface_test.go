package sftpclient

import (
	"context"
	"io"
	"os"
	"testing"
)

type mockStatReader struct {
	io.Reader
	size int64
}

func (m *mockStatReader) Stat() (os.FileInfo, error) {
	return &mockStatFileInfo{size: m.size}, nil
}

type mockStatFileInfo struct {
	os.FileInfo
	size int64
}

func (m *mockStatFileInfo) Size() int64 { return m.size }

func TestReaderInterfaceSatisfaction(t *testing.T) {
	mock := &mockStatReader{size: 12345}
	
	t.Run("progressReader", func(t *testing.T) {
		pr := &progressReader{r: mock}
		if s, ok := interface{}(pr).(interface{ Stat() (os.FileInfo, error) }); !ok {
			t.Errorf("progressReader does not implement Stat()")
		} else {
			info, _ := s.Stat()
			if info.Size() != 12345 {
				t.Errorf("wrong size from Stat: %d", info.Size())
			}
		}
		if s, ok := interface{}(pr).(interface{ Size() int64 }); ok {
			if s.Size() != 12345 {
				t.Errorf("wrong size from Size(): %d", s.Size())
			}
		} else {
			t.Errorf("progressReader does not implement Size()")
		}
	})

	t.Run("throttledReader", func(t *testing.T) {
		tr := &throttledReader{r: mock, ctx: context.Background()}
		if _, ok := interface{}(tr).(interface{ Stat() (os.FileInfo, error) }); !ok {
			t.Errorf("throttledReader does not implement Stat()")
		}
		if _, ok := interface{}(tr).(interface{ Size() int64 }); !ok {
			t.Errorf("throttledReader does not implement Size()")
		}
	})

	t.Run("contextReader", func(t *testing.T) {
		cr := &contextReader{r: mock, ctx: context.Background()}
		if _, ok := interface{}(cr).(interface{ Stat() (os.FileInfo, error) }); !ok {
			t.Errorf("contextReader does not implement Stat()")
		}
		if _, ok := interface{}(cr).(interface{ Size() int64 }); !ok {
			t.Errorf("contextReader does not implement Size()")
		}
	})
}
