package model

import "errors"

//short url
type ShortUrl string

//long url
type Url string

//site
type Site struct {
	Url      Url
	ShortUrl ShortUrl
}

//sites collection
type Sites struct {
	Data map[ShortUrl]Site
}

//errors
var (
	ErrSiteExists = errors.New("Site allready shorted")
)

func (sites *Sites) Add(url Url) error {
	if sites.Data == nil {
		sites.Data = map[ShortUrl]Site{}
	}
	//todo move to service
	short := ShortUrl("dsdsdsd")

	if _, ok := sites.Data[short]; ok {
		return ErrSiteExists
	}

	site := Site{
		Url:      url,
		ShortUrl: short,
	}
	sites.Data[short] = site
	return nil
}
