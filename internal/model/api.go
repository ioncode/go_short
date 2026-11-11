package model

// PostRequest представляет входящий запрос на сокращение одной ссылки через REST API.
type PostRequest struct {
	// URL содержит оригинальный адрес, который необходимо сократить.
	URL URL `json:"url"`
}

// ErrorResponse описывает стандартную структуру ответа сервера в случае возникновения ошибки.
type ErrorResponse struct {
	// Error содержит текстовое описание произошедшей ошибки.
	Error string `json:"error"`
}

// PostResponse представляет успешный ответ сервера, содержащий результат сокращения одной ссылки.
type PostResponse struct {
	// Result содержит готовый короткий URL.
	Result string `json:"result"`
}

// BatchPostRequestItem описывает один элемент во входящем массиве для пакетного сокращения ссылок.
type BatchPostRequestItem struct {
	// URL содержит оригинальный адрес, отправленный на сокращение.
	URL URL `json:"original_url"`
	// CorrelationID — уникальный идентификатор строки, передаваемый клиентом для связи запроса и ответа.
	CorrelationID string `json:"correlation_id"`
}

// BatchPostResponseItem описывает один элемент в исходящем массиве ответа при пакетном сокращении ссылок.
type BatchPostResponseItem struct {
	// Alias содержит готовый короткий URL для соответствующего элемента батча.
	Alias ShortURL `json:"short_url"`
	// CorrelationID возвращает переданный клиентом идентификатор для сопоставления с исходным запросом.
	CorrelationID string `json:"correlation_id"`
}

// UserSitesResponseItem описывает элемент списка всех ранее сокращенных ссылок текущего авторизованного пользователя.
type UserSitesResponseItem struct {
	// Alias содержит готовый короткий URL.
	Alias ShortURL `json:"short_url"`
	// URL содержит исходный оригинальный адрес.
	URL URL `json:"original_url"`
}
