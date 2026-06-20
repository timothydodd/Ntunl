package compress

import (
	"bytes"
	"testing"
)

func TestGzipRoundTrip(t *testing.T) {
	orig := []byte("the quick brown fox jumps over the lazy dog")
	enc, err := GzipCompress(orig)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	dec, err := GzipDecompress(enc)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(dec, orig) {
		t.Fatalf("gzip round trip mismatch")
	}
}

func TestBrotliRoundTrip(t *testing.T) {
	orig := []byte("the quick brown fox jumps over the lazy dog")
	enc, err := BrotliCompress(orig)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	dec, err := BrotliDecompress(enc)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(dec, orig) {
		t.Fatalf("brotli round trip mismatch")
	}
}

func TestCombineUrlPath(t *testing.T) {
	cases := [][3]string{
		{"http://a.com/", "/foo", "http://a.com/foo"},
		{"http://a.com", "foo", "http://a.com/foo"},
		{"http://a.com/", "foo", "http://a.com/foo"},
		{"http://a.com", "/foo", "http://a.com/foo"},
	}
	for _, c := range cases {
		if got := CombineUrlPath(c[0], c[1]); got != c[2] {
			t.Errorf("CombineUrlPath(%q,%q)=%q want %q", c[0], c[1], got, c[2])
		}
	}
}
