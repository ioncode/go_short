package benchmark

import (
	"bytes"
	"compress/gzip"
	"net/http/httptest"
	"testing"
)

func Benchmark_E2E_Batch_NoCompression(b *testing.B) {
	ctx, appRouter, _ := initBenchEnv()
	rawBuf := make([]byte, 0, 256)
	var counter uint64

	res := httptest.NewRecorder()

	for b.Loop() {
		counter++
		rawBuf = generateBatchJSON(rawBuf, counter)

		resetRecorder(res)

		executePostRequest(b, ctx, appRouter, "/api/shorten/batch", "application/json", "", "", rawBuf, res)
	}
}

func Benchmark_E2E_Batch_WithRequestCompression(b *testing.B) {
	ctx, appRouter, _ := initBenchEnv()
	rawBuf := make([]byte, 0, 256)
	gzipBuf := bytes.NewBuffer(make([]byte, 0, 512))
	gzipWriter := gzip.NewWriter(gzipBuf)
	var counter uint64

	res := httptest.NewRecorder()

	for b.Loop() {
		counter++
		gzipBuf.Reset()
		gzipWriter.Reset(gzipBuf)

		rawBuf = generateBatchJSON(rawBuf, counter)
		_, _ = gzipWriter.Write(rawBuf)
		_ = gzipWriter.Close()

		resetRecorder(res)

		executePostRequest(b, ctx, appRouter, "/api/shorten/batch", "application/json", "gzip", "", gzipBuf.Bytes(), res)
	}
}

func Benchmark_E2E_Batch_WithFullCompression(b *testing.B) {
	ctx, appRouter, _ := initBenchEnv()
	rawBuf := make([]byte, 0, 256)
	gzipBuf := bytes.NewBuffer(make([]byte, 0, 512))
	gzipWriter := gzip.NewWriter(gzipBuf)
	var counter uint64

	res := httptest.NewRecorder()

	for b.Loop() {
		counter++
		gzipBuf.Reset()
		gzipWriter.Reset(gzipBuf)

		rawBuf = generateBatchJSON(rawBuf, counter)
		_, _ = gzipWriter.Write(rawBuf)
		_ = gzipWriter.Close()

		resetRecorder(res)

		executePostRequest(b, ctx, appRouter, "/api/shorten/batch", "application/json", "gzip", "gzip", gzipBuf.Bytes(), res)
	}
}
