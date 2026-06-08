package repository_test

import (
	"testing"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/repository"
)

func TestMapRepository_GetByUrl(t *testing.T) {
	tests := []struct {
		name      string
		url       model.Url
		want      model.Site
		wantError bool
	}{
		{
			name: "Get stored site",
			url:  "ya.ru",
			want: model.Site{
				Url:      "ya.ru",
				ShortUrl: "123",
			},
			wantError: false,
		},
		{
			name: "Get unstored site",
			url:  "ya.ru1",
			want: model.Site{
				Url:      "ya.ru",
				ShortUrl: "123",
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := repository.NewMapRepository()
			r.StoreSite(tt.want)
			got, gotSite := r.GetByUrl(tt.url)
			if gotSite != true {
				if !tt.wantError {
					t.Errorf("GetByUrl() failed: %v", gotSite)
				}
				return
			}
			if tt.wantError {
				t.Fatal("GetByUrl() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("GetByUrl() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapRepository_GetByAlias(t *testing.T) {
	tests := []struct {
		name    string
		alias   model.ShortUrl
		want    model.Site
		wantErr bool
	}{
		{
			name:  "Get by stored alias",
			alias: "123",
			want: model.Site{
				Url:      "ya.ru",
				ShortUrl: "123",
			},
			wantErr: false,
		},
		{
			name:  "Get by unstored alias",
			alias: "1234",
			want: model.Site{
				Url:      "ya.ru",
				ShortUrl: "123",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := repository.NewMapRepository()
			r.StoreSite(tt.want)
			got, gotErr := r.GetByAlias(tt.alias)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("GetByAlias() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("GetByAlias() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("GetByAlias() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapRepository_StoreSite(t *testing.T) {
	tests := []struct {
		name    string
		site    model.Site
		wantErr bool
	}{
		{
			name: "Store new site",
			site: model.Site{
				Url:      "ya.ru",
				ShortUrl: "123",
			},
			wantErr: false,
		},
		{
			name: "Store allready stored site",
			site: model.Site{
				Url:      "yandex.ru",
				ShortUrl: "sfdsfgd",
			},
			wantErr: true,
		},
	}
	r := repository.NewMapRepository()
	r.StoreSite(model.Site{
		Url:      "yandex.ru",
		ShortUrl: "sfdsfgd",
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			gotErr := r.StoreSite(tt.site)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("StoreSite() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("StoreSite() succeeded unexpectedly")
			}
		})
	}
}
