package main

import (
	"context"
	"flag"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// defaultTokenFilePath is the projected service account token path used for workload identity federation.
const defaultTokenFilePath = "/var/run/secrets/eks.amazonaws.com/serviceaccount/token" //#nosec G101 not a credential

// Flags for client ID, tenant ID
var (
	clientID             = flag.String("client-id", "", "The client ID of the Azure AD application.")
	tenantID             = flag.String("tenant-id", "", "The tenant ID of the Azure AD.")
	subscriptionID       = flag.String("subscription-id", "", "The Azure subscription ID.")
	regionFilter         = flag.String("region-filter", "", "Optional region filter to limit the SKUs returned.")
	skuFilter            = flag.String("sku-filter", "", "Optional SKU filter to limit the SKUs returned.")
	restrictionFilter    = flag.String("restriction-exclude-filter", "", "Optional restriction filter to limit the SKUs returned (supported values: 'NotAvailableForSubscription', 'QuotaId').")
	minAvailabilityZones = flag.Int("min-availability-zones", 3, "Minimum number of availability zones required for an instance type to be included in the results.")
	identityTokenPath    = flag.String("identity-token-path", defaultTokenFilePath, "The file path where the identity token is located.")
)

func main() {
	flag.Parse()

	if *clientID == "" || *tenantID == "" {
		log.Fatal().Err(errors.New("both --client-id and --tenant-id must be provided")).Msg("invalid arguments")
	}

	if *subscriptionID == "" {
		log.Fatal().Err(errors.New("--subscription-id must be provided")).Msg("invalid arguments")
	}

	cred, credErr := azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
		ClientID:      *clientID,
		TenantID:      *tenantID,
		TokenFilePath: *identityTokenPath,
	})

	if credErr != nil {
		panic(credErr)
	}

	// Create the client
	skuClient, clientErr := armcompute.NewResourceSKUsClient(
		*subscriptionID, cred, nil)
	if clientErr != nil {
		panic(clientErr)
	}

	// List SKUs
	pager := skuClient.NewListPager(&armcompute.ResourceSKUsClientListOptions{
		Filter: skuFilter,
	})

	ctx := context.Background()

	type regionInfo struct {
		Name string
		AZs  []string
	}

	instanceTypeToAZsMap := make(map[string]regionInfo)
	instanceTypeToRestriction := make(map[string][]*armcompute.ResourceSKURestrictions)
	for pager.More() {
		page, pageErr := pager.NextPage(ctx)
		if pageErr != nil {
			panic(pageErr)
		}

		// Process the results
		for _, sku := range page.Value {
			if sku.LocationInfo != nil {
				// If region filter is provided, check if any location matches and skip the SKU if none do
				if *regionFilter != "" {
					matched := false
					for _, locationInfo := range sku.LocationInfo {
						if locationInfo.Location != nil && strings.EqualFold(*locationInfo.Location, *regionFilter) {
							matched = true
							break
						}
					}
					if !matched {
						continue
					}
				}

				for _, locationInfo := range sku.LocationInfo {
					if locationInfo.Zones != nil {

						// If restriction filter is provided, check if any restriction matches and skip the SKU if it does
						if *restrictionFilter != "" && sku.Restrictions != nil {
							matched := false
							for _, restriction := range sku.Restrictions {
								if restriction.Type != nil && string(*restriction.ReasonCode) == *restrictionFilter {
									matched = true
									break
								}
							}
							if matched {
								continue
							}
						}

						// If minimum availability zones is provided, check if the SKU meets the requirement
						if len(locationInfo.Zones) < *minAvailabilityZones {
							continue
						}

						instanceTypeZones := make([]string, 0)
						for _, zone := range locationInfo.Zones {
							azName := fromPtr(zone)
							instanceTypeZones = append(instanceTypeZones, azName)
						}

						// Sort AZs for deterministic results
						slices.Sort(instanceTypeZones)
						instanceTypeToAZsMap[*sku.Name] = regionInfo{
							Name: fromPtr(locationInfo.Location),
							AZs:  instanceTypeZones,
						}
						instanceTypeToRestriction[*sku.Name] = sku.Restrictions
						break
					}
				}
			}
		}
	}

	for instanceType, azs := range instanceTypeToAZsMap {
		restrictions := instanceTypeToRestriction[instanceType]
		log.Info().Str("instanceType", instanceType).Any("restrictions", restrictions).
			Any("locationInfo", azs).Msg("Instance Type Availability Zones")
	}
}

func fromPtr[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
