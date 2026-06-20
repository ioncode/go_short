package model

type PostRequest struct {
	Url Url `json:"url"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type PostResponse struct {
	Result string `json:"result"`
}
