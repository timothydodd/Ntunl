// Package compress provides gzip/brotli helpers and a small URL-path join used
// across the host and client. Ported from NtunlCommon/Utility.cs.
package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
)

// EncodeType selects a compression algorithm.
type EncodeType int

const (
	EncodeNone EncodeType = iota
	EncodeGzip
	EncodeBrotli
)

// Compress encodes data with the given algorithm, returning it unchanged for
// EncodeNone.
func Compress(data []byte, encoding EncodeType) ([]byte, error) {
	switch encoding {
	case EncodeBrotli:
		return BrotliCompress(data)
	case EncodeGzip:
		return GzipCompress(data)
	default:
		return data, nil
	}
}

func BrotliCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func GzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func BrotliDecompress(data []byte) ([]byte, error) {
	r := brotli.NewReader(bytes.NewReader(data))
	return io.ReadAll(r)
}

func GzipDecompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// CombineUrlPath joins a base path and subpath with exactly one slash between
// them, mirroring Utility.CombineUrlPath.
func CombineUrlPath(path, subpath string) string {
	return strings.TrimRight(path, "/") + "/" + strings.TrimLeft(subpath, "/")
}
