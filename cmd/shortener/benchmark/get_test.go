package benchmark

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ioncode/go_short/internal/model"
)

func Benchmark_E2E_Get_Alias(b *testing.B) {
	ctx, appRouter, repo := initBenchEnv()

	// Наполняем In-Memory БД одной записью до старта основного цикла
	targetAlias := model.ShortUrl("yandex")
	err := repo.StoreSite(model.Site{
		ShortUrl: targetAlias,
		Url:      "https://yandex.ru",
	})
	if err != nil {
		b.Fatalf("ошибка подготовки данных: %v", err)
	}

	res := httptest.NewRecorder()
	path := "/" + string(targetAlias)

	b.ResetTimer()
	for b.Loop() {
		resetRecorder(res)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			b.Fatalf("ошибка создания запроса: %v", err)
		}

		// Защищаем мидлварь авторизации от повторных аллокаций криптографии
		if authCookie != nil {
			req.AddCookie(authCookie)
		}

		appRouter.ServeHTTP(res, req)

		respResult := res.Result()
		respResult.Body.Close()

		if respResult.StatusCode != http.StatusTemporaryRedirect {
			b.Fatalf("Ожидался статус 307, получен %d. Ответ: %s", respResult.StatusCode, res.Body.String())
		}
		benchSink = res
	}
}

// Benchmark_E2E_GetUserSites замеряет скорость получения списка ссылок текущего пользователя
func Benchmark_E2E_GetUserSites(b *testing.B) {
	ctx, appRouter, repo := initBenchEnv()

	// Наполняем базу тестовыми ссылками, ПРИВЯЗАННЫМИ к глобальному benchUserID
	if benchUserID == "" {
		b.Fatal("критическая ошибка: benchUserID не был инициализирован")
	}

	for i := 1; i <= 5; i++ {
		idxStr := strconv.Itoa(i)
		err := repo.StoreSite(model.Site{
			ShortUrl: model.ShortUrl("alias_" + idxStr),
			Url:      model.Url("https://yandex.ru" + idxStr),
			UserId:   benchUserID, // Идеальное совпадение UUID!
		})
		if err != nil {
			b.Fatalf("ошибка наполнения базы: %v", err)
		}
	}

	res := httptest.NewRecorder()

	for b.Loop() {
		resetRecorder(res)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/user/urls", nil)
		if err != nil {
			b.Fatalf("ошибка создания запроса: %v", err)
		}

		if authCookie != nil {
			req.AddCookie(authCookie)
		}

		appRouter.ServeHTTP(res, req)

		respResult := res.Result()
		respResult.Body.Close()

		if respResult.StatusCode != http.StatusOK {
			b.Fatalf("Ожидался статус 200 OK, получен %d. Ответ: %s", respResult.StatusCode, res.Body.String())
		}
		benchSink = res
	}
}
