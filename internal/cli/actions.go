package cli

import (
	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/httpaction"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/shellaction"
)

// configuredActions composes only opt-in trusted built-ins at the CLI boundary.
func configuredActions(cfg config.Config) (*action.Runtime, registry.Catalog, error) {
	entries, err := httpaction.Registrations(cfg.HTTP)
	if err != nil {
		return nil, nil, err
	}
	shell, err := shellaction.Registrations(cfg.Shell)
	if err != nil {
		return nil, nil, err
	}
	entries = append(entries, shell...)
	var grants []string
	bindings := map[string]string{}
	for _, entry := range entries {
		grants = append(grants, entry.Descriptor.Permissions...)
		bindings[entry.Descriptor.ID] = entry.Descriptor.Version
	}
	runtime, err := action.New(entries, grants)
	if err != nil {
		return nil, nil, err
	}
	catalog, err := runtime.Catalog(bindings)
	return runtime, catalog, err
}
