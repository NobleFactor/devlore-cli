// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// region SUPPORTING TYPES

// ViperConfig carries what [InitViper] needs to set up a program's settings: its name, its environment prefix, and
// where its configuration file is.
type ViperConfig struct {

	// Name is the program's name, such as "lore" or "writ". It is required.
	Name string

	// EnvPrefix is the prefix of the program's environment variables, such as "WRIT". When it is empty, it is Name
	// upper-cased with each `-` as `_`, so devlore-test's is "DEVLORE_TEST".
	EnvPrefix string

	// ConfigName is the configuration file's name without its extension (default: "config").
	ConfigName string

	// ConfigType is the configuration file's type (default: "yaml").
	ConfigType string

	// UseSharedConfig reads the shared configuration file, `~/.config/devlore/config.yaml`, where each program's
	// settings sit under its name, instead of a file of the program's own.
	UseSharedConfig bool
}

// endregion

// region EXPORTED FUNCTIONS

// BindFlags binds each persistent flag of `cmd` to the setting of the same name.
//
// With the shared configuration file, a flag's setting sits under the program's name: `--repo` binds `writ.repo`.
// Without it, the setting is the flag's name alone: `--repo` binds `repo`.
//
// Parameters:
//   - `cmd`: the command whose persistent flags are bound; the shared root passes the program's root.
//   - `toolName`: the program's name, such as "writ".
//   - `useSharedConfig`: whether the program reads the shared configuration file.
//
// Returns:
//   - `error`: non-nil when viper refuses to bind a flag.
func BindFlags(cmd *cobra.Command, toolName string, useSharedConfig bool) error {

	var bindErr error

	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if bindErr != nil {
			return
		}

		key := f.Name
		if useSharedConfig {
			key = toolName + "." + f.Name
		}

		if err := viper.BindPFlag(key, f); err != nil {
			bindErr = fmt.Errorf("failed to bind flag %s: %w", f.Name, err)
		}
	})

	return bindErr
}

// InitViper sets up a program's settings: its configuration file, and the environment variables that override it.
//
// Precedence, lowest to highest: the configuration file, then the environment, then the command line. The shared root
// calls it before any command runs.
//
// A setting's environment variable is the program's prefix plus the setting's key without the program's own segment,
// upper-cased, with each `.` and `-` as `_`: `writ.repo` reads `WRIT_REPO`, `writ.deploy.conflict` reads
// `WRIT_DEPLOY_CONFLICT`, and `devlore-test.dry-run` reads `DEVLORE_TEST_DRY_RUN`. The doubled spelling,
// `WRIT_WRIT_REPO`, names nothing. A map read whole, such as `writ.vars`, comes from the configuration file alone: no
// variable reaches its entries.
//
// Parameters:
//   - `cfg`: the program's name and prefix, and where its configuration file is.
//
// Returns:
//   - `error`: non-nil when `cfg.Name` is empty, or when the configuration file exists and cannot be read.
func InitViper(cfg ViperConfig) error {

	if cfg.Name == "" {
		return errors.New("ViperConfig.Name is required")
	}

	if cfg.EnvPrefix == "" {
		cfg.EnvPrefix = strings.ReplaceAll(strings.ToUpper(cfg.Name), "-", "_")
	}
	if cfg.ConfigName == "" {
		cfg.ConfigName = "config"
	}
	if cfg.ConfigType == "" {
		cfg.ConfigType = "yaml"
	}

	viper.SetConfigName(cfg.ConfigName)
	viper.SetConfigType(cfg.ConfigType)

	if cfg.UseSharedConfig {
		viper.AddConfigPath(devlore.ConfigHome())
	} else {
		viper.AddConfigPath(xdg.ConfigPath(cfg.Name))
	}

	bindEnvironment(viper.GetViper(), cfg.EnvPrefix, cfg.Name)

	// A missing configuration file is not an error: the settings come from the environment, the flags, and their
	// defaults.
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("error reading config: %w", err)
		}
	}

	return nil
}

// SharedConfigPath returns the path of the shared configuration file, `~/.config/devlore/config.yaml`.
//
// Returns:
//   - `string`: the file's path, whether or not it exists.
func SharedConfigPath() string {
	return devlore.ConfigPath("config.yaml")
}

// endregion

// region HELPER FUNCTIONS

// bindEnvironment makes `settings` read each setting from its environment variable, named as [InitViper] states.
//
// Parameters:
//   - `settings`: the viper instance to configure; [InitViper] passes the process-wide one.
//   - `prefix`: the program's environment prefix, such as "WRIT".
//   - `program`: the program's name, such as "writ"; a key that begins with its segment reads the variable without it.
func bindEnvironment(settings *viper.Viper, prefix, program string) {

	settings.SetEnvPrefix(prefix)
	settings.AutomaticEnv()
	settings.SetEnvKeyReplacer(environmentKeyReplacer(prefix, program))
}

// environmentKeyReplacer returns the replacer that turns a setting's prefixed key into its environment variable.
//
// viper upper-cases `<prefix>_<key>` before it replaces, so a key under the program's own segment arrives as
// `<PREFIX>_<PROGRAM>.<REST>`, the doubled prefix leading the name. The first pair turns that doubled prefix into
// `<PREFIX>_`; the others turn each `.` and `-` into `_`.
//
// Parameters:
//   - `prefix`: the program's environment prefix, such as "WRIT".
//   - `program`: the program's name, such as "writ".
//
// Returns:
//   - `*strings.Replacer`: the replacer viper applies to each name it looks up.
func environmentKeyReplacer(prefix, program string) *strings.Replacer {
	return strings.NewReplacer(prefix+"_"+strings.ToUpper(program)+".", prefix+"_", ".", "_", "-", "_")
}

// endregion
