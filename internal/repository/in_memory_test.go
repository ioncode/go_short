package repository_test

import (
	"os"
	"testing"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/repository"
)

func TestMapRepository_GetByUrl(t *testing.T) {
	tests := []struct {
		name      string
		url       model.URL
		want      model.Site
		wantError bool
	}{
		{
			name: "Get stored site",
			url:  "ya.ru",
			want: model.Site{
				URL:      "ya.ru",
				ShortURL: "123",
			},
			wantError: false,
		},
		{
			name: "Get unstored site",
			url:  "ya.ru1",
			want: model.Site{
				URL:      "ya.ru",
				ShortURL: "123",
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := repository.NewMapRepository("test_storage.json")
			t.Cleanup(func() {
				_ = r.Close()
				_ = os.RemoveAll("test_storage.json")
			})
			_, _ = r.StoreSite(tt.want)
			got, gotError := r.GetByURL(tt.url)
			if gotError != nil {
				if !tt.wantError {
					t.Errorf("GetByUrl() failed: %v", gotError)
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
		alias   model.ShortURL
		want    model.Site
		wantErr bool
	}{
		{
			name:  "Get by stored alias",
			alias: "123",
			want: model.Site{
				URL:      "ya.ru",
				ShortURL: "123",
			},
			wantErr: false,
		},
		{
			name:  "Get by unstored alias",
			alias: "1234",
			want: model.Site{
				URL:      "ya.ru",
				ShortURL: "123",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := repository.NewMapRepository("test_storage.json")
			t.Cleanup(func() {
				_ = r.Close()
				_ = os.RemoveAll("test_storage.json")
			})
			_, _ = r.StoreSite(tt.want)
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
				URL:      "ya.ru",
				ShortURL: "123",
			},
			wantErr: false,
		},
		{
			name: "Store allready stored site",
			site: model.Site{
				URL:      "yandex.ru",
				ShortURL: "sfdsfgd",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			r := repository.NewMapRepository("test_storage.json")
			t.Cleanup(func() {
				_ = r.Close()
				_ = os.RemoveAll("test_storage.json")
			})
			_, _ = r.StoreSite(model.Site{
				URL:      "yandex.ru",
				ShortURL: "sfdsfgd",
			})

			_, gotErr := r.StoreSite(tt.site)
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
