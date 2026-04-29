package ui

import (
	"embed"
	"encoding/json"
	"net/http"
	"proxy-ai/internal/config"
)

//go:embed index.html
var uiFS embed.FS

func ServeUI(w http.ResponseWriter, r *http.Request) {
	data, err := uiFS.ReadFile("index.html")
	if err != nil {
		http.Error(w, "UI not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func HandleGetConfig(cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func HandleSaveConfig(cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// Update in-memory config
	cfg.Port = newCfg.Port
	cfg.DefaultProfile = newCfg.DefaultProfile
	cfg.Profiles = newCfg.Profiles

	// Save to disk
	config.Save(cfg)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
