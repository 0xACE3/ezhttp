package ezhttp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"

	"github.com/andybalholm/brotli"
)

// ThroughFunc transforms response body bytes. Used with Response.Through().
type ThroughFunc func([]byte) ([]byte, error)

// DecodeBase64 decodes base64-encoded body.
func DecodeBase64(b []byte) ([]byte, error) {
	return base64.StdEncoding.DecodeString(string(b))
}

// DecodeGzip decompresses gzip-encoded body.
func DecodeGzip(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// DecodeBrotli decompresses brotli-encoded body.
func DecodeBrotli(b []byte) ([]byte, error) {
	return io.ReadAll(brotli.NewReader(bytes.NewReader(b)))
}

// Chain composes multiple ThroughFuncs into one, applied left to right.
func Chain(fns ...ThroughFunc) ThroughFunc {
	return func(b []byte) ([]byte, error) {
		var err error
		for _, fn := range fns {
			b, err = fn(b)
			if err != nil {
				return nil, err
			}
		}
		return b, nil
	}
}
