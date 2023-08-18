package repoutils

import (
	"fmt"
	"log"
	"strings"

	"github.com/distribution/distribution/reference"
	"github.com/docker/cli/cli/config"
	clitypes "github.com/docker/cli/cli/config/types"
	types "github.com/docker/docker/api/types/registry"
)

const latestTagSuffix = ":latest"

// GetAuthConfig returns the docker registry AuthConfig.
// Optionally takes in the authentication values, otherwise pulls them from the
// docker config file.
func GetAuthConfig(username, password, registry string) (types.AuthConfig, error) {
	if username != "" && password != "" && registry != "" {
		return types.AuthConfig{
			Username:      username,
			Password:      password,
			ServerAddress: registry,
		}, nil
	}

	dcfg, err := config.Load(config.Dir())
	if err != nil {
		return types.AuthConfig{}, fmt.Errorf("loading config file failed: %v", err)
	}

	// if they passed a specific registry, return those creds _if_ they exist
	if registry != "" {
		var tryRegistry []string

		if strings.HasPrefix(registry, "https://") {
			tryRegistry = append(tryRegistry, strings.TrimPrefix(registry, "https://"))
		} else if strings.HasPrefix(registry, "http://") {
			tryRegistry = append(tryRegistry, strings.TrimPrefix(registry, "http://"))
		} else {
			tryRegistry = append(tryRegistry, registry)
			tryRegistry = append(tryRegistry, "https://"+registry)
		}

		for _, registryCleaned := range tryRegistry {
			creds, err := dcfg.GetAuthConfig(registryCleaned)
			if err == nil {
				c := fixAuthConfig(creds, registryCleaned)
				return c, nil
			}
		}
	}

	// Don't use any authentication.
	// We should never get here.
	log.Println("Not using any authentication")
	return types.AuthConfig{}, nil
}

// fixAuthConfig overwrites the AuthConfig's ServerAddress field with the
// registry value if ServerAddress is empty. For example, config.Load() will
// return AuthConfigs with empty ServerAddresses if the configuration file
// contains only an "credsHelper" object.
func fixAuthConfig(creds clitypes.AuthConfig, registry string) (c types.AuthConfig) {
	c.Username = creds.Username
	c.Password = creds.Password
	c.Auth = creds.Auth
	c.Email = creds.Email
	c.IdentityToken = creds.IdentityToken
	c.RegistryToken = creds.RegistryToken

	c.ServerAddress = creds.ServerAddress
	if creds.ServerAddress == "" {
		c.ServerAddress = registry
	}

	return c
}

// GetRepoAndRef parses the repo name and reference.
func GetRepoAndRef(image string) (repo, ref string, err error) {
	if image == "" {
		return "", "", reference.ErrNameEmpty
	}

	image = addLatestTagSuffix(image)

	var parts []string
	if strings.Contains(image, "@") {
		parts = strings.Split(image, "@")
	} else if strings.Contains(image, ":") {
		parts = strings.Split(image, ":")
	}

	repo = parts[0]
	if len(parts) > 1 {
		ref = parts[1]
	}

	return
}

// addLatestTagSuffix adds :latest to the image if it does not have a tag
func addLatestTagSuffix(image string) string {
	if !strings.Contains(image, ":") {
		return image + latestTagSuffix
	}
	return image
}
