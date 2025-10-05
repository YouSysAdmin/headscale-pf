package main

import (
	"os"

	"github.com/yousysadmin/headscale-pf/internal/sources"
	"github.com/yousysadmin/headscale-pf/pkg"
	term_color "github.com/yousysadmin/headscale-pf/pkg/term-color"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	inputPolicyFile        string
	outputPolicyFile       string
	source                 string
	endpoint               string
	token                  string
	ldapBindPassword       string
	ldapBindDN             string
	ldapBaseDN             string
	ldapDefaultEmailDomain string
	keycloakRealm          string

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
	cliCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable color output")
	cliCmd.PersistentFlags().BoolVar(&stripEmailDomain, "strip-email-domain", true, "Strip e-mail domain")

	cliCmd.PersistentFlags().StringVar(&source, "source", os.Getenv("PF_SOURCE"), "Source (can use env var PF_SOURCE)")
	cliCmd.PersistentFlags().StringVar(&endpoint, "endpoint", os.Getenv("PF_ENDPOINT"), "Source endpoint (can use env var PF_ENDPOINT)")
	cliCmd.PersistentFlags().StringVar(&token, "token", os.Getenv("PF_TOKEN"), "A provider API token (can use env var PF_TOKEN)")

	// Specific flags for the LDAP source
	cliCmd.PersistentFlags().StringVar(&ldapBaseDN, "ldap-base-dn", os.Getenv("PF_LDAP_BASE_DN"), "Base DN to use for LDAP searches (can use env var PF_LDAP_BASE_DN)")
	cliCmd.PersistentFlags().StringVar(&ldapBindDN, "ldap-bind-dn", os.Getenv("PF_LDAP_BIND_DN"), "Distinguished Name of the LDAP bind user account (can use env var PF_LDAP_BIND_DN)")
	cliCmd.PersistentFlags().StringVar(&ldapDefaultEmailDomain, "ldap-default-email-domain", os.Getenv("PF_LDAP_DEFAULT_EMAIL_DOMAIN"),
		"Default email domain to append when user entries lack a mail attribute (can use env var PF_LDAP_DEFAULT_USER_EMAIL_DOMAIN)",
	)
	cliCmd.PersistentFlags().StringVar(&ldapBindPassword, "ldap-bind-password", os.Getenv("PF_LDAP_BIND_PASSWORD"), "LDAP password (can use env var PF_LDAP_BIND_PASSWORD)")

	// Specifc flags for the Keycloak source
	cliCmd.PersistentFlags().StringVar(&keycloakRealm, "keycloak-realm", os.Getenv("PF_KEYCLOAK_REALM"), "Keycloak Realm (can use env var PF_KEYCLOAK_REALM)")

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
		// Make logger channel and start output
		logCh := make(chan string)
		defer close(logCh)

		go func() {
			for ls := range logCh {
				logger.Info(ls)
			}
		}()

		// Make a new client
		client, err := sources.NewSource(sources.SourceConfig{
			Name:                   source,
			Token:                  token,
			Endpoint:               endpoint,
			LDAPBindPassword:       ldapBindPassword,
			LDAPBindDN:             ldapBindDN,
			LDAPBaseDN:             ldapBaseDN,
			LDAPDefaultEmailDomain: ldapDefaultEmailDomain,
			KeycloakRealm:          keycloakRealm,
		})
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
