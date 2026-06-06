package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapTileProxyFetchesAndCaches(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/3/1/2.png" {
			t.Fatalf("upstream path = %q, want /3/1/2.png", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("tile-data"))
	}))
	defer upstream.Close()

	row, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Tiles", URLTemplate: upstream.URL + "/{z}/{x}/{y}.png", MaxZoom: 18, Enabled: true})
	if err != nil {
		t.Fatalf("CreateMapTileSource() error = %v", err)
	}

	cacheDir := t.TempDir()
	router := newRouter(webConfig{StaticDir: t.TempDir(), MapTileCacheDir: cacheDir}, st, nil, nil, nil, nil, nil)

	url := "/api/map/" + row.URLTemplateHash + "?x=1&y=2&z=3"
	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, body = %s", i+1, recorder.Code, recorder.Body.String())
		}
		if recorder.Body.String() != "tile-data" {
			t.Fatalf("request %d body = %q, want tile-data", i+1, recorder.Body.String())
		}
	}
	if requests != 1 {
		t.Fatalf("upstream requests = %d, want 1", requests)
	}

	cachePath := filepath.Join(cacheDir, row.URLTemplateHash, "3", "1", "2.tile")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file %s: %v", cachePath, err)
	}
	if string(data) != "tile-data" {
		t.Fatalf("cache file = %q, want tile-data", string(data))
	}
}

func TestMapTileProxyRejectsInvalidCoordinates(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	row, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Tiles", URLTemplate: "https://tiles.example.com/{z}/{x}/{y}.png", MaxZoom: 3, Enabled: true})
	if err != nil {
		t.Fatalf("CreateMapTileSource() error = %v", err)
	}

	router := newRouter(webConfig{StaticDir: t.TempDir(), MapTileCacheDir: t.TempDir()}, st, nil, nil, nil, nil, nil)

	cases := []string{
		"/api/map/" + row.URLTemplateHash + "?y=0&z=0",
		"/api/map/" + row.URLTemplateHash + "?x=-1&y=0&z=0",
		"/api/map/" + row.URLTemplateHash + "?x=0&y=0&z=4",
		"/api/map/" + row.URLTemplateHash + "?x=2&y=0&z=1",
	}
	for _, url := range cases {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400; body = %s", url, recorder.Code, recorder.Body.String())
		}
	}
}

func TestMapTileProxyUnknownAndDisabledSource(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	disabled, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Disabled", URLTemplate: "https://disabled.example.com/{z}/{x}/{y}.png", MaxZoom: 3, Enabled: false})
	if err != nil {
		t.Fatalf("CreateMapTileSource() error = %v", err)
	}

	router := newRouter(webConfig{StaticDir: t.TempDir(), MapTileCacheDir: t.TempDir()}, st, nil, nil, nil, nil, nil)

	cases := []string{
		"/api/map/not-a-hash?x=0&y=0&z=0",
		"/api/map/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa?x=0&y=0&z=0",
		"/api/map/" + disabled.URLTemplateHash + "?x=0&y=0&z=0",
	}
	wantStatus := []int{http.StatusBadRequest, http.StatusNotFound, http.StatusNotFound}
	for i, url := range cases {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != wantStatus[i] {
			t.Fatalf("%s status = %d, want %d; body = %s", url, recorder.Code, wantStatus[i], recorder.Body.String())
		}
	}
}

func TestMapTileProxyUpstreamStatus(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/404/") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "upstream error", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	row404, err := st.CreateMapTileSource(mapTileSourceInput{Name: "NotFoundTiles", URLTemplate: upstream.URL + "/404/{z}/{x}/{y}.png", MaxZoom: 18, Enabled: true})
	if err != nil {
		t.Fatalf("CreateMapTileSource(404) error = %v", err)
	}
	row500, err := st.CreateMapTileSource(mapTileSourceInput{Name: "StatusTiles", URLTemplate: upstream.URL + "/{z}/{x}/{y}.png", MaxZoom: 18, Enabled: true})
	if err != nil {
		t.Fatalf("CreateMapTileSource(500) error = %v", err)
	}

	router := newRouter(webConfig{StaticDir: t.TempDir(), MapTileCacheDir: t.TempDir()}, st, nil, nil, nil, nil, nil)

	cases := []struct {
		url  string
		want int
	}{
		{url: "/api/map/" + row404.URLTemplateHash + "?x=0&y=0&z=0", want: http.StatusNotFound},
		{url: "/api/map/" + row500.URLTemplateHash + "?x=0&y=0&z=0", want: http.StatusBadGateway},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != tc.want {
			t.Fatalf("%s status = %d, want %d; body = %s", tc.url, recorder.Code, tc.want, recorder.Body.String())
		}
	}
}
