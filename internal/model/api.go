package model

type PostRequest struct {
	URL Url `json:"url"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type PostResponse struct {
	Result string `json:"result"`
}

type BatchPostRequestItem struct {
	URL           Url    `json:"original_url"`
	CorrelationId string `json:"correlation_id"`
}

type BatchPostResponseItem struct {
	Alias         ShortUrl `json:"short_url"`
	CorrelationId string   `json:"correlation_id"`
}

type UserSitesResponseItem struct {
	Alias ShortUrl `json:"short_url"`
	URL   Url      `json:"original_url"`
}
