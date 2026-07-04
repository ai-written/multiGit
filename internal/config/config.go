package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Registry            string            `json:"registry"`
	Packages            []string          `json:"packages"`
	PackageVersions     map[string]string `json:"packageVersions"`
	PackageVersionCache map[string]string `json:"packageVersionCache"`
	AutoBumpProjects    []string          `json:"autoBumpProjects"`
	RootPath            string            `json:"rootPath"`
	UpdateRepo          string            `json:"updateRepo"`
	CommitCount         int               `json:"commitCount"`
}

func DefaultConfig() Config {
	return Config{
		Registry:            "https://registry.npmmirror.com",
		Packages:            []string{},
		PackageVersions:     map[string]string{},
		PackageVersionCache: map[string]string{},
		AutoBumpProjects:    []string{},
		RootPath:            "",
		UpdateRepo:          "ai-written/multiGit",
		CommitCount:         5,
	}
}

func configDir() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		appData = filepath.Join(home, ".config")
	}
	return filepath.Join(appData, "MultiGit"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := Save(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	if cfg.Registry == "" {
		cfg.Registry = "https://registry.npmmirror.com"
	}
	if cfg.Packages == nil {
		cfg.Packages = []string{}
	}
	if cfg.PackageVersions == nil {
		cfg.PackageVersions = map[string]string{}
	}
	if cfg.PackageVersionCache == nil {
		cfg.PackageVersionCache = map[string]string{}
	}
	if cfg.AutoBumpProjects == nil {
		cfg.AutoBumpProjects = []string{}
	}
	if cfg.CommitCount == 0 {
		cfg.CommitCount = 5
	}

	return cfg, nil
}

func Save(cfg Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
