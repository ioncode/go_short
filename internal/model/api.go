package model

type PostRequest struct {
	URL URL `json:"url"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type PostResponse struct {
	Result string `json:"result"`
}

type BatchPostRequestItem struct {
	URL           URL    `json:"original_url"`
	CorrelationID string `json:"correlation_id"`
}

type BatchPostResponseItem struct {
	Alias         ShortURL `json:"short_url"`
	CorrelationID string   `json:"correlation_id"`
}

type UserSitesResponseItem struct {
	Alias ShortURL `json:"short_url"`
	URL   URL      `json:"original_url"`
}
