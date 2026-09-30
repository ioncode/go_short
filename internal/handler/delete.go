package handler

import (
	"net/http"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/service"
	"github.com/ioncode/go_short/pkg"
	"github.com/ioncode/httpcodec"
)

// DeleteService определяет интерфейс для постановки задач на асинхронное удаление ссылок.
type DeleteService interface {
	Enqueue(task service.DeleteTask)
}

// AsyncDeleteUserSites обрабатывает HTTP-запросы DELETE /api/user/urls на пакетное удаление ссылок.
//
// Метод полностью оптимизирован: вычитка и десериализация JSON-массива алиасов
// переведены на метод codec.ReadJSON из моей Open-Source библиотеки httpcodec.
// Это предотвращает фрагментацию памяти и снижает нагрузку на Garbage Collector при массовых удалениях.
func AsyncDeleteUserSites(s DeleteService, codec *httpcodec.Codec) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			writeJSONError(res, "Ошибка авторизации", http.StatusUnauthorized, codec)
			return
		}

		var aliases []model.ShortUrl

		if !codec.ReadJSON(res, req, &aliases) {
			return // Ошибки формата или DoS-атак уже отправлены кодеком наружу
		}

		// Передаем очищенный срез алиасов в асинхронный воркер-пул через очередь
		s.Enqueue(service.DeleteTask{
			Author:  *user,
			Aliases: aliases,
		})

		// Возвращаем стандартный статус 202 Accepted согласно ТЗ
		res.WriteHeader(http.StatusAccepted)
	}
}
