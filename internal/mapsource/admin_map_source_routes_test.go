package mapsource

import (
	"testing"

	storepkg "meshtastic_mqtt_server/internal/store"
)

func TestIsExternalTileURLTemplate(t *testing.T) {
	for _, in := range []string{"http://tile.openstreetmap.org/x", "https://webst03.is.autonavi.com/appmaptile?key=1", "HTTPS://EXAMPLE.COM/x"} {
		if !isExternalTileURLTemplate(in) {
			t.Errorf("%q should be external", in)
		}
	}
	for _, in := range []string{"/api/map/abc?x={x}", "", "  /relative/path  "} {
		if isExternalTileURLTemplate(in) {
			t.Errorf("%q should not be external", in)
		}
	}
}

func TestPublicDTOAlwaysProxiesExternalURL(t *testing.T) {
	row := storepkg.MapTileSourceRecord{
		URLTemplate: "https://tile.example.com/style=7&x={x}&y={y}&z={z}&key=SECRET_KEY",
	}
	dto := PublicDTO(row)
	got, _ := dto["url_template"].(string)
	want := "/api/map/" + storepkg.MapTileSourceHash(row.URLTemplate) + "?x={x}&y={y}&z={z}"
	if got != want {
		t.Errorf("url_template = %q, want %q", got, want)
	}
	if dto["url_template"].(string) == row.URLTemplate {
		t.Error("external url_template must not be exposed")
	}
}

func TestPublicDTOKeepsRelativeTemplate(t *testing.T) {
	row := storepkg.MapTileSourceRecord{
		URLTemplate: "/api/map/abc?x={x}&y={y}&z={z}",
	}
	dto := PublicDTO(row)
	if got := dto["url_template"].(string); got != row.URLTemplate {
		t.Errorf("relative template should stay as-is, got %q", got)
	}
}
