package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type RepoConfig struct {
	Name     string            `yaml:"name"`
	Owner    string            `yaml:"owner"`
	Branch   string            `yaml:"branch"`
	Labels   []string          `yaml:"labels"`
	Reviewers []string         `yaml:"reviewers"`
}

type MultiRepoConfig struct {
	Repos []RepoConfig `yaml:"repos"`
}

type ConfigPlugin struct{}

func (p *ConfigPlugin) Execute(env *Envelop, agent interface{}) *Envelop {
	if env.Intent != "load_config" {
		env.Receiver = "github-webhook"
		return env
	}

	repo, _ := env.Payload["repo"].(string)
	if repo == "" {
		env.Receiver = "github-webhook"
		return env
	}

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		env.Receiver = "github-webhook"
		return env
	}

	var config MultiRepoConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		env.Receiver = "github-webhook"
		return env
	}

	for _, r := range config.Repos {
		if r.Name == repo {
			env.Meta["current_repo_config"] = r
			break
		}
	}

	env.Payload["config_loaded"] = true
	env.Payload["repo_count"] = len(config.Repos)

	readyEnv := &Envelop{
		Sender:   "config",
		Receiver: "grp.config",
		Intent:   "config_ready",
		Payload:  map[string]interface{}{"repo": repo},
		Meta:     env.Meta,
		TTL:      env.TTL,
	}

	bus := getBus()
	if bus != nil {
		bus.Publish("grp.config", readyEnv)
	}

	env.Receiver = "github-webhook"
	return env
}

var busInstance *Bus

func getBus() *Bus {
	return busInstance
}

func SetBus(b *Bus) {
	busInstance = b
}

func (p *ConfigPlugin) LoadConfigFromFile(path string) (*MultiRepoConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config MultiRepoConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}