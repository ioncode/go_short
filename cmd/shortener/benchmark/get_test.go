package benchmark

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ioncode/go_short/internal/model"
)

func Benchmark_E2E_Get_Alias(b *testing.B) {
	env := initBenchEnv(b)

	targetAlias := model.ShortUrl("yandex")
	err := env.Repo.StoreSite(model.Site{
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
		req, err := http.NewRequestWithContext(env.Ctx, http.MethodGet, path, nil)
		if err != nil {
			b.Fatalf("ошибка запроса: %v", err)
		}
		if env.Cookie != nil {
			req.AddCookie(env.Cookie)
		}

		env.Router.ServeHTTP(res, req)

		respResult := res.Result()
		respResult.Body.Close()
		if respResult.StatusCode != http.StatusTemporaryRedirect {
			b.Fatalf("Ожидался статус 307, получен %d", respResult.StatusCode)
		}
		benchSink = res
	}
}

func Benchmark_E2E_GetUserSites(b *testing.B) {
	env := initBenchEnv(b)

	if env.UserID == "" {
		b.Fatal("критическая ошибка: UserId не был инициализирован")
	}

	// База чистая! Вставка пройдет мгновенно за O(1)
	for i := 1; i <= 5; i++ {
		idxStr := strconv.Itoa(i)
		err := env.Repo.StoreSite(model.Site{
			ShortUrl: model.ShortUrl("get_user_alias_" + idxStr),
			Url:      model.Url("https://get-user-sites.com" + idxStr),
			UserId:   env.UserID,
		})
		if err != nil {
			b.Fatalf("ошибка наполнения базы: %v", err)
		}
	}

	res := httptest.NewRecorder()

	b.ResetTimer()
	for b.Loop() {
		resetRecorder(res)
		req, err := http.NewRequestWithContext(env.Ctx, http.MethodGet, "/api/user/urls", nil)
		if err != nil {
			b.Fatalf("ошибка запроса: %v", err)
		}
		if env.Cookie != nil {
			req.AddCookie(env.Cookie)
		}

		env.Router.ServeHTTP(res, req)

		respResult := res.Result()
		respResult.Body.Close()

		if respResult.StatusCode != http.StatusOK {
			b.Fatalf("Ожидался статус 200 OK, получен %d. Ответ: %s", respResult.StatusCode, res.Body.String())
		}
		benchSink = res
	}
}
