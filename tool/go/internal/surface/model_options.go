package surface

import (
	"errors"
	"fmt"
	"strings"
)

// ProviderSettings records the user's selections, not the model reported by a provider.
type ProviderSettings struct {
	RequestedModel string `json:"requestedModel"`
	CLIEffort      string `json:"cliEffort,omitempty"`
}

func hasProviderOptions(args Args) bool {
	return args.Model != "" || args.FallbackModel != "" || args.Effort != "" || args.FallbackEffort != ""
}

// providerOptions resolves role-based flags once, without modifying persisted settings.
func providerOptions(cfg Config, args Args, providers []string) (Config, map[string]ProviderSettings, error) {
	if len(providers) < 2 && (args.FallbackModel != "" || args.FallbackEffort != "") {
		return nil, nil, errors.New("fallback model and effort options require a distinct fallback provider")
	}
	resolved := Config(mergeObject(map[string]any(cfg), nil))
	agents := mergeObject(object(cfg["agents"]), nil)
	resolved["agents"] = agents
	settings := make(map[string]ProviderSettings, len(providers))
	requireModel := args.Command == "run" || (args.Command == "doctor" && args.Live)
	for i, provider := range providers {
		model, effort, flag := args.Model, args.Effort, "--model"
		if i == 1 {
			model, effort, flag = args.FallbackModel, args.FallbackEffort, "--fallback-model"
		}
		agent := mergeObject(object(agents[provider]), nil)
		agents[provider] = agent
		if model != "" {
			agent["model"] = model
		}
		requested, _ := agent["model"].(string)
		if requireModel && strings.TrimSpace(requested) == "" {
			return nil, nil, fmt.Errorf("model for %s is required; use %s <id> or agents.%s.model in .refactor/config.json", provider, flag, provider)
		}
		settings[provider] = ProviderSettings{RequestedModel: requested, CLIEffort: effort}
	}
	return resolved, settings, nil
}
