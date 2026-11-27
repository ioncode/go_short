package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestValueOrDefault(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want string
	}{
		{
			name: "пустое значение возвращает N/A",
			val:  "",
			want: "N/A",
		},
		{
			name: "непустое значение возвращается без изменений",
			val:  "v1.0.0",
			want: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := valueOrDefault(tt.val); got != tt.want {
				t.Errorf("valueOrDefault(%q) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestPrintBuildInfo(t *testing.T) {
	// Сохраняем исходные значения глобальных переменных и восстанавливаем их после теста
	origVersion, origDate, origCommit := buildVersion, buildDate, buildCommit
	t.Cleanup(func() {
		buildVersion = origVersion
		buildDate = origDate
		buildCommit = origCommit
	})

	tests := []struct {
		name    string
		version string
		date    string
		commit  string
		want    string
	}{
		{
			name:    "значения по умолчанию (N/A)",
			version: "",
			date:    "",
			commit:  "",
			want:    "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:    "переданные через ldflags значения",
			version: "v1.2.3",
			date:    "2026-11-27T10:00:00Z",
			commit:  "a8b5cf8",
			want:    "Build version: v1.2.3\nBuild date: 2026-11-27T10:00:00Z\nBuild commit: a8b5cf8\n",
		},
		{
			name:    "частично заполненные значения",
			version: "v0.1.0",
			date:    "",
			commit:  "deadbeef",
			want:    "Build version: v0.1.0\nBuild date: N/A\nBuild commit: deadbeef\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buildVersion = tt.version
			buildDate = tt.date
			buildCommit = tt.commit

			got := captureStdout(t, printBuildInfo)
			if got != tt.want {
				t.Errorf("printBuildInfo() вывел:\n%q\nожидалось:\n%q", got, tt.want)
			}
		})
	}
}

// captureStdout подменяет os.Stdout на pipe, выполняет fn и возвращает весь перехваченный вывод.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("не удалось создать os.Pipe: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("не удалось закрыть writer pipe: %v", err)
	}
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("не удалось прочитать вывод из pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("не удалось закрыть reader pipe: %v", err)
	}

	return buf.String()
}
