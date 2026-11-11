// Пакет staticlint представляет собой кастомный инструмент статического анализа (multichecker).
// Он объединяет стандартные анализаторы Go,
// проверки из пакета staticcheck.io, сторонние публичные анализаторы и собственное правило
// для запрета прямого вызова os.Exit.
//
// # Механизм запуска мультичекера
//
// Утилита компилируется в исполняемый файл и запускается через терминал аналогично стандартному `go vet`.
// Вы можете проверять как отдельные файлы, так и весь проект рекурсивно.
//
// Примеры команд для сборки и запуска:
//
//	# Сборка анализатора
//	go build -o staticlint ./cmd/staticlint/main.go
//
//	# Запуск проверки для всего текущего проекта (включая подпакеты)
//	./staticlint ./...
//
//	# Запуск конкретного анализатора (например, только правила для os.Exit)
//	./staticlint -osexit ./...
//
// # Описание включенных анализаторов
//
// 1. Стандартные анализаторы (golang.org/x/tools/go/analysis/passes):
//   - В сборку автоматически импортируются все базовые анализаторы. Например:
//   - printf: Проверяет соответствие строк форматирования и аргументов в функциях типа Printf.
//   - shadow: Находит возможное непреднамеренное затенение переменных во внутренних блоках.
//   - structtag: Проверяет синтаксическую корректность тегов у структур (json, xml и т.д.).
//   - copylock: Находит ошибочную передачу мьютексов (sync.Mutex) по значению.
//
// 2. Анализаторы из пакета staticcheck.io:
//   - Все проверки класса SA: Направлены на поиск багов, дефектов логики и явных ошибок корректности.
//   - Проверка ST1000 (класс ST): Контролирует наличие и корректность документации пакетов.
//   - Проверка S1000 (класс S): Предлагает варианты упрощения кода (Code Simplifications).
//   - Проверка QF1001 (класс QF): Дает рекомендации по быстрому рефакторингу (Quick Fixes).
//
// 3. Сторонние публичные анализаторы:
//   - github.com/kisielk/errcheck/errcheck: Проверяет, что возвращаемые функциями ошибки (error) не игнорируются.
//   - github.com/timakin/bodyclose/passes/bodyclose: Проверяет, что дескрипторы `res.Body` в HTTP-запросах закрываются вовремя через defer.
//
// 4. Собственный анализатор (osexit):
//   - Запрещает использовать прямой вызов функции `os.Exit` внутри функции `main` пакета `main`.
//   - Игнорирует вызовы os.Exit в тестовых файтах (*_test.go) и в любых других функциях, кроме main.main.
package main

import (
	"go/ast"
	"strings"

	"github.com/kisielk/errcheck/errcheck"
	bodycloseanalyzer "github.com/timakin/bodyclose/passes/bodyclose"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/appends"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/atomicalign"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/cgocall"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/analysis/passes/deepequalerrors"
	"golang.org/x/tools/go/analysis/passes/defers"
	"golang.org/x/tools/go/analysis/passes/directive"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/fieldalignment"
	"golang.org/x/tools/go/analysis/passes/findcall"
	"golang.org/x/tools/go/analysis/passes/framepointer"
	"golang.org/x/tools/go/analysis/passes/gofix"
	"golang.org/x/tools/go/analysis/passes/hostport"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"
	"golang.org/x/tools/go/analysis/passes/inline"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/pkgfact"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/reflectvaluecompare"
	"golang.org/x/tools/go/analysis/passes/scannererr"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/sigchanyzer"
	"golang.org/x/tools/go/analysis/passes/slog"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/sqlrowserr"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/stdversion"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/timeformat"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unsafeptr"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"golang.org/x/tools/go/analysis/passes/unusedwrite"
	"golang.org/x/tools/go/analysis/passes/usesgenerics"
	"golang.org/x/tools/go/analysis/passes/waitgroup"
	"honnef.co/go/tools/staticcheck"
)

// OsExitAnalyzer — собственный анализатор, запрещающий os.Exit в main.main.
var OsExitAnalyzer = &analysis.Analyzer{
	Name: "osexit",
	Doc:  "restricts direct calls to os.Exit in main function of main package",
	Run:  runOsExit,
}

func main() {
	var mychecks []*analysis.Analyzer

	// 1. Добавляем все стандартные статические анализаторы из passes
	mychecks = append(mychecks,
		appends.Analyzer, asmdecl.Analyzer, assign.Analyzer, atomic.Analyzer,
		atomicalign.Analyzer, bools.Analyzer, buildssa.Analyzer, buildtag.Analyzer,
		cgocall.Analyzer, composite.Analyzer, copylock.Analyzer, ctrlflow.Analyzer,
		deepequalerrors.Analyzer, defers.Analyzer, directive.Analyzer, errorsas.Analyzer,
		fieldalignment.Analyzer, findcall.Analyzer, framepointer.Analyzer, gofix.Analyzer,
		hostport.Analyzer, httpresponse.Analyzer, ifaceassert.Analyzer, inline.Analyzer,
		inspect.Analyzer, loopclosure.Analyzer, lostcancel.Analyzer,
		nilfunc.Analyzer, nilness.Analyzer, pkgfact.Analyzer, printf.Analyzer,
		reflectvaluecompare.Analyzer, scannererr.Analyzer, shadow.Analyzer, shift.Analyzer,
		sigchanyzer.Analyzer, slog.Analyzer, sortslice.Analyzer, sqlrowserr.Analyzer,
		stdmethods.Analyzer, stdversion.Analyzer, stringintconv.Analyzer, structtag.Analyzer,
		testinggoroutine.Analyzer, tests.Analyzer, timeformat.Analyzer, unmarshal.Analyzer,
		unreachable.Analyzer, unsafeptr.Analyzer, unusedresult.Analyzer, unusedwrite.Analyzer,
		usesgenerics.Analyzer, waitgroup.Analyzer,
	)

	// 2. Добавляем анализаторы из staticcheck.io
	for _, v := range staticcheck.Analyzers {
		// Все анализаторы класса SA
		if strings.HasPrefix(v.Analyzer.Name, "SA") {
			mychecks = append(mychecks, v.Analyzer)
		}
		// Не менее одного анализатора остальных классов (добавим точечно ST1000, S1000, QF1001)
		if v.Analyzer.Name == "ST1000" || v.Analyzer.Name == "S1000" || v.Analyzer.Name == "QF1001" {
			mychecks = append(mychecks, v.Analyzer)
		}
	}

	// 3. Добавляем два публичных сторонних линтера
	mychecks = append(mychecks, errcheck.Analyzer)          // Проверяет необработанные ошибки
	mychecks = append(mychecks, bodycloseanalyzer.Analyzer) // Проверяет закрытие http-body (использует алиас)

	// 4. Добавляем свой собственный кастомный линтер
	mychecks = append(mychecks, OsExitAnalyzer)

	// Точка входа для запуска всех зарегистрированных проверок
	multichecker.Main(mychecks...)
}

// Логика собственного анализатора os.Exit
func runOsExit(pass *analysis.Pass) (any, error) {
	// Проверяем только пакет main
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		// Игнорируем тестовые файлы, так как в них os.Exit легален (например, в TestMain)
		filename := pass.Fset.File(file.Pos()).Name()
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		// Обходим AST-дерево текущего файла
		ast.Inspect(file, func(node ast.Node) bool {
			// Ищем объявления функций
			funcDecl, ok := node.(*ast.FuncDecl)
			if !ok {
				return true
			}

			// Нас интересует только функция с именем "main"
			if funcDecl.Name.Name != "main" {
				return true
			}

			// Внутри функции main ищем выражения вызова функций
			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				callExpr, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Проверяем, является ли вызываемый объект селектором (пакет.Функция)
				selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				// Проверяем, что имя пакета "os", а имя функции "Exit"
				if ident, ok := selExpr.X.(*ast.Ident); ok && ident.Name == "os" && selExpr.Sel.Name == "Exit" {
					pass.Reportf(callExpr.Pos(), "direct call to os.Exit in main function is prohibited")
				}

				return true
			})

			return true
		})
	}

	return nil, nil
}
