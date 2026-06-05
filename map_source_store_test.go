package main

import (
	"errors"
	"testing"
)

func TestMapTileSourceDefaultSeeded(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	row, err := st.GetDefaultMapTileSource()
	if err != nil {
		t.Fatalf("GetDefaultMapTileSource() error = %v", err)
	}
	if row.Name != defaultMapTileSourceName || row.URLTemplate != defaultMapTileSourceURLTemplate || !row.Enabled || !row.IsDefault {
		t.Fatalf("default map source = %+v, want built-in default", row)
	}
}

func TestCreateMapTileSourceValidation(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if _, err := st.CreateMapTileSource(mapTileSourceInput{Name: "bad", URLTemplate: "https://tiles.example.com/{z}/{x}.png", MaxZoom: 19, Enabled: true}); err == nil {
		t.Fatal("CreateMapTileSource() missing placeholder error = nil, want error")
	}
	if _, err := st.CreateMapTileSource(mapTileSourceInput{Name: "bad", URLTemplate: "javascript:alert(1)/{z}/{x}/{y}", MaxZoom: 19, Enabled: true}); err == nil {
		t.Fatal("CreateMapTileSource() invalid scheme error = nil, want error")
	}
	if _, err := st.CreateMapTileSource(mapTileSourceInput{Name: "bad", URLTemplate: "https://user:pass@tiles.example.com/{z}/{x}/{y}.png", MaxZoom: 19, Enabled: true}); err == nil {
		t.Fatal("CreateMapTileSource() credentials error = nil, want error")
	}
}

func TestListEnabledMapTileSources(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	disabled, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Disabled", URLTemplate: "https://disabled.example.com/{z}/{x}/{y}.png", MaxZoom: 18, Enabled: false})
	if err != nil {
		t.Fatalf("CreateMapTileSource(disabled) error = %v", err)
	}
	custom, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Custom", URLTemplate: "https://custom.example.com/{z}/{x}/{y}.png", MaxZoom: 18, Enabled: true})
	if err != nil {
		t.Fatalf("CreateMapTileSource(custom) error = %v", err)
	}
	if _, err := st.SetDefaultMapTileSource(custom.ID); err != nil {
		t.Fatalf("SetDefaultMapTileSource() error = %v", err)
	}

	rows, err := st.ListEnabledMapTileSources()
	if err != nil {
		t.Fatalf("ListEnabledMapTileSources() error = %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("ListEnabledMapTileSources() length = %d, want at least 2", len(rows))
	}
	if rows[0].ID != custom.ID {
		t.Fatalf("first enabled source id = %d, want default %d", rows[0].ID, custom.ID)
	}
	for _, row := range rows {
		if row.ID == disabled.ID {
			t.Fatalf("disabled source was returned: %+v", row)
		}
		if !row.Enabled {
			t.Fatalf("disabled row returned: %+v", row)
		}
	}
}

func TestMapTileSourceDuplicateAndDefaultRules(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	first, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Custom", URLTemplate: "https://tiles.example.com/{z}/{x}/{y}.png", MaxZoom: 18, Enabled: true})
	if err != nil {
		t.Fatalf("CreateMapTileSource() error = %v", err)
	}
	if _, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Custom", URLTemplate: "https://tiles2.example.com/{z}/{x}/{y}.png", MaxZoom: 18, Enabled: true}); !errors.Is(err, errMapTileSourceAlreadyExists) {
		t.Fatalf("duplicate name error = %v, want errMapTileSourceAlreadyExists", err)
	}
	if _, err := st.CreateMapTileSource(mapTileSourceInput{Name: "Custom 2", URLTemplate: first.URLTemplate, MaxZoom: 18, Enabled: true}); !errors.Is(err, errMapTileSourceAlreadyExists) {
		t.Fatalf("duplicate url error = %v, want errMapTileSourceAlreadyExists", err)
	}

	updated, err := st.SetDefaultMapTileSource(first.ID)
	if err != nil {
		t.Fatalf("SetDefaultMapTileSource() error = %v", err)
	}
	if !updated.IsDefault {
		t.Fatalf("updated default = %+v, want is_default", updated)
	}

	oldDefault, err := st.GetDefaultMapTileSource()
	if err != nil {
		t.Fatalf("GetDefaultMapTileSource() error = %v", err)
	}
	if oldDefault.ID != first.ID {
		t.Fatalf("default id = %d, want %d", oldDefault.ID, first.ID)
	}
	if _, err := st.UpdateMapTileSource(first.ID, mapTileSourceInput{Name: first.Name, URLTemplate: first.URLTemplate, Attribution: first.Attribution, MaxZoom: first.MaxZoom, Enabled: false, IsDefault: true}); !errors.Is(err, errMapTileSourceCannotDisableDefault) {
		t.Fatalf("disable default error = %v, want errMapTileSourceCannotDisableDefault", err)
	}
	if err := st.DeleteMapTileSource(first.ID); !errors.Is(err, errMapTileSourceCannotDeleteDefault) {
		t.Fatalf("delete default error = %v, want errMapTileSourceCannotDeleteDefault", err)
	}
}
