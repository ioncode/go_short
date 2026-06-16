package model

//short url
type ShortUrl string

//long url
type Url string

//site
type Site struct {
	Url      Url
	ShortUrl ShortUrl
}
