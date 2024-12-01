package main

import (
	"github.com/yousysadmin/headscale-pf/internal/sources"
	"github.com/yousysadmin/headscale-pf/pkg"
	"github.com/yousysadmin/headscale-pf/pkg/term-color"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	inputPolicyFile  string
	outputPolicyFile string
	token            string
	username         string
	underpass        string
	logger           *pterm.Logger
	noColor          bool
	stripEmailDomain bool

	cliCmd = &cobra.Command{
		Use:     "headscale-pf",
		Short:   "headscale-pf - fills groups in policy",
		Long:    `Obtaining information about groups and group members from external sources and populating groups in the Headscale policy.`,
		Version: pkg.Version,
	}
)

func init() {
	// Add command persistent flag
	cliCmd.PersistentFlags().StringVar(&inputPolicyFile, "input-policy", "./policy.hjson", "Headscale policy file template")
	cliCmd.PersistentFlags().StringVar(&outputPolicyFile, "output-policy", "./current.json", "Headscale prepared policy file")
	cliCmd.PersistentFlags().StringVar(&token, "token", os.Getenv("PF_TOKEN"), "A provider API token (can use env var PF_TOKEN)")
	cliCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable color output")
	cliCmd.PersistentFlags().BoolVar(&stripEmailDomain, "strip-email-domain", true, "Strip e-mail domain")

	// In the future, for other sources with basic auth, etc. :)
	cliCmd.PersistentFlags().StringVar(&username, "user", os.Getenv("PF_USER_NAME"), "A provider API user (can use env var PF_USER_NAME)")
	cliCmd.PersistentFlags().StringVar(&underpass, "password", os.Getenv("PF_USER_PASSWORD"), "A provider API user password (can use env var PF_USER_PASSWORD)")

	// Disable colors if terminal doesn't support or user set flag --no-color
	if !term_color.CheckTerminalColorSupport() || noColor {
		pterm.DisableColor()
	}

	// Configure logger
	logger = pterm.DefaultLogger.
		WithLevel(pterm.LogLevelInfo).
		WithMaxWidth(120).
		WithTime(false)

	// Add commands
	cliCmd.AddCommand(prepare)
}

// Prepare
var prepare = &cobra.Command{
	Use:     "prepare",
	Short:   "Prepare policy",
	Aliases: []string{"p"},
	Run: func(cmd *cobra.Command, args []string) {
		if cmd.Flag("token").Value.String() == "" {
			errorInfo := map[string]any{
				"Error": "token is required",
			}
			logger.Fatal("Prepare error:", logger.ArgsFromMap(errorInfo))
		}

		// Make logger channel and start output
		logCh := make(chan string)
		defer close(logCh)

		go func() {
			for ls := range logCh {
				logger.Info(ls)
			}
		}()

		// Make a new client
		client, err := sources.NewSource(sources.SourceConfig{Name: "jc", Token: token})
		if err != nil {
			errorInfo := map[string]any{
				"Error": err.Error(),
			}
			logger.Fatal("Source error:", logger.ArgsFromMap(errorInfo))
		}

		// Obtain users from a remote source and fill policy
		if err := preparePolicy(client, logCh); err != nil {
			errorInfo := map[string]any{
				"Error": err.Error(),
			}
			logger.Fatal("Prepare error:", logger.ArgsFromMap(errorInfo))
		}
	},
}
