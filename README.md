## Azure SKU Tool

Quick tool to scan for instance types eligible for use under certain conditions
- Subscription
- Region
- Restrictions
- Min. Availability Zones


### Usage

**Running inside dp-agent (RECOMMENDED):**

```bash
kubectl -n dataplane-agent exec -it deployment/dp-agent -- bash
skutool --tenant-id $AZURE_TENANT_ID --subscription-id $AZURE_SUBSCRIPTION_ID --client-id $AZURE_CLIENT_ID -identity-token-path /var/run/secrets/azure/tokens/azure-identity-token -restriction-exclude-filter NotAvailableForSubscription
```

**Running from any host with a workload identity token:**

```bash
skutool --client-id <CLIENT ID> --tenant-id <TENANT ID> --subscription-id <SUBSCRIPTION ID> --sku-filter "location eq <REGION>" --restriction-exclude-filter "NotAvailableForSubscription" --min-availability-zones 3
```

### Flags

```bash
Usage of skutool:
  -alsologtostderr
    	log to standard error as well as files
  -client-id string
    	The client ID of the Azure AD application.
  -identity-token-path string
    	The file path where the identity token is located. (default "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -log_link string
    	If non-empty, add symbolic links in this directory to the log files
  -logbuflevel int
    	Buffer log messages logged at this level or lower (-1 means don't buffer; 0 means buffer INFO only; ...). Has limited applicability on non-prod platforms.
  -logtostderr
    	log to standard error instead of files
  -min-availability-zones int
    	Minimum number of availability zones required for an instance type to be included in the results. (default 3)
  -region-filter string
    	Optional region filter to limit the SKUs returned.
  -restriction-exclude-filter string
    	Optional restriction filter to limit the SKUs returned (supported values: 'NotAvailableForSubscription', 'QuotaId').
  -sku-filter string
    	Optional SKU filter to limit the SKUs returned.
  -stderrthreshold value
    	logs at or above this threshold go to stderr (default 2)
  -subscription-id string
    	The Azure subscription ID.
  -tenant-id string
    	The tenant ID of the Azure AD.
```

### Install

Download a prebuilt binary for your platform from the [releases page](https://github.com/omnistrate-community/azure-sku-tool/releases):

```bash
curl -L -o skutool https://github.com/omnistrate-community/azure-sku-tool/releases/latest/download/skutool-linux-arm64
chmod +x skutool
```

Available binaries: `skutool-linux-amd64`, `skutool-linux-arm64`, `skutool-darwin-amd64`, `skutool-darwin-arm64`.

### Build

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o skutool ./
```
