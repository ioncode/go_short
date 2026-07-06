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
