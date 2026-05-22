package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/config"
)

// Environment variables expected at runtime:
//
//	CF_API_URL    - CF API endpoint, e.g. https://api.cf.eu10.hana.ondemand.com
//	              - login and token URLs are discovered automatically from this endpoint
//	GITHUB_JWT    - GitHub Actions OIDC token, already exchanged by the workflow step
//	CF_ORIGIN     - UAA origin (identity provider alias), required for custom IdPs on SAP BTP
//	CF_ORG_NAME   - CF organisation name to target
//	CF_SPACE_NAME - CF space name to target
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	apiURL := mustEnv("CF_API_URL")
	assertion := mustEnv("GITHUB_JWT")
	origin := mustEnv("CF_ORIGIN")
	orgName := mustEnv("CF_ORG_NAME")
	spaceName := mustEnv("CF_SPACE_NAME")

	// Build the config using the JWT Bearer assertion. Login and token URLs are
	// discovered automatically from the CF API root endpoint, so only the API
	// URL is required. The assertion is exchanged with UAA once here; subsequent
	// API calls use the resulting refresh token.
	cfg, err := config.New(apiURL,
		config.JWTBearerAssertion(assertion),
		config.Origin(origin),
	)
	if err != nil {
		// A bad or expired assertion surfaces here, not on the first API call.
		return fmt.Errorf("config.New (jwt-bearer exchange): %w", err)
	}

	cf, err := client.New(cfg)
	if err != nil {
		return fmt.Errorf("creating CF client: %w", err)
	}

	return listApps(ctx, cf, orgName, spaceName)
}

func listApps(ctx context.Context, cf *client.Client, orgName, spaceName string) error {
	orgs, err := cf.Organizations.ListAll(ctx, nil)
	if err != nil {
		return fmt.Errorf("listing orgs: %w", err)
	}
	var orgGUID string
	for _, o := range orgs {
		if o.Name == orgName {
			orgGUID = o.GUID
			break
		}
	}
	if orgGUID == "" {
		return fmt.Errorf("org %q not found", orgName)
	}

	spaceOpts := client.NewSpaceListOptions()
	spaceOpts.Names.EqualTo(spaceName)
	spaceOpts.OrganizationGUIDs.EqualTo(orgGUID)
	spaces, err := cf.Spaces.ListAll(ctx, spaceOpts)
	if err != nil {
		return fmt.Errorf("listing spaces: %w", err)
	}
	if len(spaces) == 0 {
		return fmt.Errorf("space %q not found in org %q", spaceName, orgName)
	}
	spaceGUID := spaces[0].GUID

	appOpts := client.NewAppListOptions()
	appOpts.SpaceGUIDs.EqualTo(spaceGUID)
	apps, err := cf.Applications.ListAll(ctx, appOpts)
	if err != nil {
		return fmt.Errorf("listing apps: %w", err)
	}

	if len(apps) == 0 {
		fmt.Printf("No apps found in %s / %s\n", orgName, spaceName)
		return nil
	}

	fmt.Printf("Apps in org=%s space=%s:\n\n", orgName, spaceName)
	fmt.Printf("%-40s  %-10s\n", "NAME", "STATE")
	fmt.Printf("%-40s  %-10s\n", "----", "-----")
	for _, app := range apps {
		fmt.Printf("%-40s  %-10s\n", app.Name, app.State)
	}
	return nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return v
}
