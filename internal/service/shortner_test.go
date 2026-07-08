package service_test

import (
	"os"
	"testing"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/repository"
	"github.com/ioncode/go_short/internal/service"
)

type MockSiteRepository struct {
}

func TestMapShortner_Short(t *testing.T) {

	repo := repository.NewMapRepository("test_storage.json")
	t.Cleanup(func() {
		repo.Close()
		os.RemoveAll("test_storage.json")
	})
	repo.StoreSite(model.Site{
		Url:      "ya.ru",
		ShortUrl: "123",
	})
	tests := []struct {
		name    string
		r       service.SiteRepository
		Url     model.Url
		want    model.ShortUrl
		wantErr bool
	}{
		{
			name:    "Service with existing site",
			Url:     "ya.ru",
			want:    "123",
			wantErr: false,
			r:       repo,
		},
		//todo add new sites after refactoring string generation for mocking
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := service.NewShortner(tt.r)
			got, gotErr := s.Short(tt.Url)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Short() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Short() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("Short() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapShortner_Get(t *testing.T) {
	repo := repository.NewMapRepository("test_storage.json")
	t.Cleanup(func() {
		repo.Close()
		os.RemoveAll("test_storage.json")
	})
	site := model.Site{
		Url:      "ya.ru",
		ShortUrl: "123",
	}
	repo.StoreSite(site)
	tests := []struct {
		name    string
		r       service.SiteRepository
		alias   model.ShortUrl
		want    model.Site
		wantErr bool
	}{
		{
			name:    "Get stored site",
			r:       repo,
			alias:   "123",
			want:    site,
			wantErr: false,
		},
		{
			name:    "Error getting unstored site",
			r:       repo,
			alias:   "456",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := service.NewShortner(tt.r)
			got, gotErr := s.Get(tt.alias)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Get() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Get() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}
