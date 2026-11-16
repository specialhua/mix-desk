package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// settings are the app's preferences, kept as JSON in the user data
// directory. Secrets (the session token) live in the Keychain instead.
type settings struct {
	// Server is the root URL of the Mix Space server, without the API path:
	// https://server.example.com
	Server string `json:"server"`
	// Appearance is "system", "light" or "dark", for the native UI and the
	// web admin together.
	Appearance string `json:"appearance"`
	// Lang is "zh-CN" or "en-US", the web admin's locales.
	Lang string `json:"lang"`
	// Typewriter keeps the line being written in the middle of a write
	// page (writing.go).
	Typewriter bool `json:"typewriter,omitempty"`
	// Theme is the ID of the color theme (themes.go), "" for the app's
	// own colors; ThemeScope is where it applies, "write" or "global".
	Theme      string `json:"theme,omitempty"`
	ThemeScope string `json:"themeScope,omitempty"`
	// IdeaShortcut is the global shortcut of the 想法一瞬 panel
	// (idea_panel.go): "" for the default, "off" for none.
	IdeaShortcut string `json:"ideaShortcut,omitempty"`
	// SidebarTab is the sidebar's tab: 0 the blog (the web admin's pages),
	// 1 the extras (the app's own features).
	SidebarTab int `json:"sidebarTab,omitempty"`
}

func settingsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func loadSettings() settings {
	s := settings{Appearance: "system", Lang: "zh-CN"}
	path, err := settingsPath()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	if s.Appearance == "" {
		s.Appearance = "system"
	}
	if s.Lang != "en-US" {
		s.Lang = "zh-CN"
	}
	return s
}

func (s settings) save() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func themeSource(appearance string) mygo.ThemeSource {
	switch appearance {
	case "light":
		return mygo.ThemeLight
	case "dark":
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}
