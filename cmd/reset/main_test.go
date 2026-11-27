package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetGeneratorEndToEnd(t *testing.T) {
	// 1. Создаём изолированную временную директорию для тестового модуля
	tempDir := t.TempDir()

	// 2. Создаём go.mod во временной директории
	goModContent := "module testproject\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("не удалось создать go.mod: %v", err)
	}

	// 3. Создаём файл с тестовыми структурами (с маркером и без)
	modelsDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("не удалось создать директорию models: %v", err)
	}

	sourceCode := `package models

type NoResetStruct struct {
	Value int
}

// generate:reset
type ChildStruct struct {
	Name  string
	Flags []bool
}

// generate:reset
type MainStruct struct {
	IntVal      int
	FloatVal    float64
	StrVal      string
	BoolVal     bool
	StrPtr      *string
	IntPtr      *int
	NilPtr      *string
	SliceVal    []int
	MapVal      map[string]int
	ChildVal    ChildStruct
	ChildPtr    *ChildStruct
	NilChildPtr *ChildStruct
	Ignored     NoResetStruct
}
`
	sourcePath := filepath.Join(modelsDir, "models.go")
	if err := os.WriteFile(sourcePath, []byte(sourceCode), 0644); err != nil {
		t.Fatalf("не удалось записать models.go: %v", err)
	}

	// 4. Запускаем генератор на временной директории
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"reset", tempDir}

	main()

	// 5. Проверяем, что файл reset.gen.go создан
	genPath := filepath.Join(modelsDir, "reset.gen.go")
	genBytes, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatalf("файл reset.gen.go не был сгенерирован: %v", err)
	}

	genStr := string(genBytes)
	if strings.Contains(genStr, "func (rs *NoResetStruct) Reset()") {
		t.Errorf("метод Reset() не должен генерироваться для структур без // generate:reset")
	}

	// 6. Создаём тест внутри временного проекта для проверки поведения Reset() в рантайме
	runtimeTestCode := `package models

import "testing"

func TestGeneratedResetBehavior(t *testing.T) {
	str := "hello"
	num := 42

	obj := &MainStruct{
		IntVal:   100,
		FloatVal: 3.14,
		StrVal:   "test",
		BoolVal:  true,
		StrPtr:   &str,
		IntPtr:   &num,
		NilPtr:   nil,
		SliceVal: []int{1, 2, 3, 4},
		MapVal:   map[string]int{"a": 1, "b": 2},
		ChildVal: ChildStruct{
			Name:  "child_val",
			Flags: []bool{true, false},
		},
		ChildPtr: &ChildStruct{
			Name:  "child_ptr",
			Flags: []bool{true},
		},
		NilChildPtr: nil,
		Ignored:     NoResetStruct{Value: 999},
	}

	// Сохраняем исходную ёмкость слайса (capacity)
	origCap := cap(obj.SliceVal)

	// Вызываем сгенерированный метод
	obj.Reset()

	// Проверяем примитивы
	if obj.IntVal != 0 || obj.FloatVal != 0 || obj.StrVal != "" || obj.BoolVal != false {
		t.Errorf("примитивы не сбросились: %+v", obj)
	}

	// Проверяем указатели на примитивы
	if obj.StrPtr == nil || *obj.StrPtr != "" {
		t.Errorf("StrPtr должен остаться не nil, но содержать пустую строку")
	}
	if obj.IntPtr == nil || *obj.IntPtr != 0 {
		t.Errorf("IntPtr должен остаться не nil, но содержать 0")
	}
	if obj.NilPtr != nil {
		t.Errorf("NilPtr должен остаться nil")
	}

	// Проверяем слайс (длина 0, capacity сохранена)
	if obj.SliceVal == nil || len(obj.SliceVal) != 0 || cap(obj.SliceVal) != origCap {
		t.Errorf("SliceVal должен обрезаться до [:0] с сохранением capacity: len=%d, cap=%d", len(obj.SliceVal), cap(obj.SliceVal))
	}

	// Проверяем мапу
	if obj.MapVal == nil || len(obj.MapVal) != 0 {
		t.Errorf("MapVal должна быть очищена, но не nil")
	}

	// Проверяем вложенные структуры с методом Reset()
	if obj.ChildVal.Name != "" || len(obj.ChildVal.Flags) != 0 {
		t.Errorf("ChildVal не сбросился: %+v", obj.ChildVal)
	}
	if obj.ChildPtr == nil || obj.ChildPtr.Name != "" || len(obj.ChildPtr.Flags) != 0 {
		t.Errorf("ChildPtr не сбросился: %+v", obj.ChildPtr)
	}
	if obj.NilChildPtr != nil {
		t.Errorf("NilChildPtr должен остаться nil")
	}

	// Проверяем безопасность вызова Reset() на nil-указателе
	var nilObj *MainStruct
	nilObj.Reset() // не должно вызывать панику
}
`
	runtimeTestPath := filepath.Join(modelsDir, "reset_behavior_test.go")
	if err = os.WriteFile(runtimeTestPath, []byte(runtimeTestCode), 0644); err != nil {
		t.Fatalf("не удалось записать reset_behavior_test.go: %v", err)
	}

	// 7. Запускаем go test внутри сгенерированного проекта
	var output []byte
	cmd := exec.Command("go", "test", "-v", "./...")
	cmd.Dir = tempDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("сгенерированный код не прошёл тесты:\n%s\nошибка: %v", string(output), err)
	}
}
