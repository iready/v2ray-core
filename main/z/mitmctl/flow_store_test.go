package mitmctl

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestDecodeGzipContentEncoding(t *testing.T) {
	plain := []byte(`{"data":{"nick_name":"zouyq"},"code":200}`)
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()

	got := decodeContentEncoding("gzip", raw)
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q want %q", got, plain)
	}

	// 漏标头但有 gzip magic
	got2 := decodeContentEncoding("", raw)
	if !bytes.Equal(got2, plain) {
		t.Fatalf("sniff got %q", got2)
	}

	out, trunc := storeBodyForView(map[string][]string{"Content-Encoding": {"gzip"}}, raw)
	if trunc || !bytes.Equal(out, plain) {
		t.Fatalf("store: trunc=%v body=%q", trunc, out)
	}
	preview := previewBody(out)
	if !strings.Contains(preview, "nick_name") {
		t.Fatalf("preview %q", preview)
	}
}
