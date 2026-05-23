package main

import (
	"os"
	"strconv"

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
	insecureSkipTLSVerify  bool
	ldapBindPassword       string
	ldapBindDN             string
	ldapBaseDN             string
	ldapDefaultEmailDomain string
	keycloakRealm          string

	logger  *pterm.Logger
	noColor bool

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
	cliCmd.PersistentFlags().StringVar(&outputPolicyFile, "output-policy", "./current.hjson", "Headscale prepared policy file")
	cliCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable color output")

	cliCmd.PersistentFlags().StringVar(&source, "source", "", "Source (can use env var PF_SOURCE)")
	cliCmd.PersistentFlags().StringVar(&endpoint, "endpoint", "", "Source endpoint (can use env var PF_ENDPOINT)")
	cliCmd.PersistentFlags().StringVar(&token, "token", "", "A provider API token (can use env var PF_TOKEN)")
	cliCmd.PersistentFlags().BoolVar(&insecureSkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS certificate verification for HTTPS/LDAPS/StartTLS (can use env var PF_INSECURE_SKIP_TLS_VERIFY)")

	// Specific flags for the LDAP source
	cliCmd.PersistentFlags().StringVar(&ldapBaseDN, "ldap-base-dn", "", "Base DN to use for LDAP searches (can use env var PF_LDAP_BASE_DN)")
	cliCmd.PersistentFlags().StringVar(&ldapBindDN, "ldap-bind-dn", "", "Distinguished Name of the LDAP bind user account (can use env var PF_LDAP_BIND_DN)")
	cliCmd.PersistentFlags().StringVar(&ldapDefaultEmailDomain, "ldap-default-email-domain", "",
		"Default email domain to append when user entries lack a mail attribute (can use env var PF_LDAP_DEFAULT_EMAIL_DOMAIN)",
	)
	cliCmd.PersistentFlags().StringVar(&ldapBindPassword, "ldap-bind-password", "", "LDAP password (can use env var PF_LDAP_BIND_PASSWORD)")

	// Specifc flags for the Keycloak source
	cliCmd.PersistentFlags().StringVar(&keycloakRealm, "keycloak-realm", "", "Keycloak Realm (can use env var PF_KEYCLOAK_REALM)")

	// Configure logger
	logger = pterm.DefaultLogger.
		WithLevel(pterm.LogLevelInfo).
		WithMaxWidth(120).
		WithTime(false)

	// Apply env-var fallbacks for any flag the user did not pass on the
	// command line. Order: explicit flag > env var > zero value. Also
	// disable colors here (after flag parsing) so --no-color takes effect.
	cliCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		applyEnvDefault(cmd, "source", &source, "PF_SOURCE")
		applyEnvDefault(cmd, "endpoint", &endpoint, "PF_ENDPOINT")
		applyEnvDefault(cmd, "token", &token, "PF_TOKEN")
		applyEnvDefault(cmd, "ldap-base-dn", &ldapBaseDN, "PF_LDAP_BASE_DN")
		applyEnvDefault(cmd, "ldap-bind-dn", &ldapBindDN, "PF_LDAP_BIND_DN")
		applyEnvDefault(cmd, "ldap-bind-password", &ldapBindPassword, "PF_LDAP_BIND_PASSWORD")
		applyEnvDefault(cmd, "ldap-default-email-domain", &ldapDefaultEmailDomain, "PF_LDAP_DEFAULT_EMAIL_DOMAIN")
		applyEnvDefault(cmd, "keycloak-realm", &keycloakRealm, "PF_KEYCLOAK_REALM")
		if !cmd.Flags().Changed("insecure-skip-tls-verify") {
			insecureSkipTLSVerify = envBool("PF_INSECURE_SKIP_TLS_VERIFY")
		}

		if !term_color.CheckTerminalColorSupport() || noColor {
			pterm.DisableColor()
		}
	}

	// Add commands
	cliCmd.AddCommand(prepare)
}

// applyEnvDefault sets *target to the value of envName when the user did not
// pass --flagName on the command line. This keeps explicit-flag-wins
// precedence while keeping env-var values out of cobra's --help output.
func applyEnvDefault(cmd *cobra.Command, flagName string, target *string, envName string) {
	if cmd.Flags().Changed(flagName) {
		return
	}
	if v := os.Getenv(envName); v != "" {
		*target = v
	}
}

// envBool parses a bool from the named env var. Empty/unset returns false.
func envBool(name string) bool {
	v := os.Getenv(name)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

// Prepare
var prepare = &cobra.Command{
	Use:     "prepare",
	Short:   "Prepare policy",
	Aliases: []string{"p"},
	Run: func(cmd *cobra.Command, args []string) {
		// Make logger channel and start output
		logCh := make(chan string, 100)
		done := make(chan struct{})

		go func() {
			for ls := range logCh {
				logger.Info(ls)
			}
			close(done)
		}()
		defer func() {
			close(logCh)
			<-done
		}()

		// Make a new client
		client, err := sources.NewSource(sources.SourceConfig{
			Name:                   source,
			Token:                  token,
			Endpoint:               endpoint,
			InsecureSkipTLSVerify:  insecureSkipTLSVerify,
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
