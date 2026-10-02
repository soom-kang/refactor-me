package controller

import (
	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
)

func providerSettings(c surface.Context) map[string]surface.ProviderSettings {
	settings := make(map[string]surface.ProviderSettings, len(c.Providers))
	for _, name := range c.Providers {
		if value, ok := c.ProviderSettings[name]; ok {
			settings[name] = value
		} else {
			settings[name] = surface.ProviderSettings{RequestedModel: surface.ConfigString(c.Config, "agents", name, "model")}
		}
	}
	return settings
}

func engineConfigForContext(c surface.Context) engine.Config {
	config := engineConfig(c.Config)
	config.EffortOverrides = make(map[string]string)
	for name, settings := range providerSettings(c) {
		if settings.CLIEffort != "" {
			config.EffortOverrides[name] = settings.CLIEffort
		}
	}
	return config
}
