package storage

import (
	"io"
	"os"
)

type diskPartReader struct {
	f *os.File
	r *io.LimitedReader
}

func newFileLimitedReader(fpath string, offset int64, size int64) (io.ReadCloser, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}

	_, err = f.Seek(offset, io.SeekStart)
	if err != nil {
		return nil, err
	}

	return &diskPartReader{
		f: f,
		r: &io.LimitedReader{
			R: f,
			N: size,
		},
	}, nil
}

func (r *diskPartReader) Close() error {
	return r.f.Close()
}

func (r *diskPartReader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}
