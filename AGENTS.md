# Agent Guide: skutool

`skutool` lists the Azure compute SKUs a subscription can use in a region. It filters them by
availability-zone count and restriction reason. Use it to answer questions like "which VM sizes
can I deploy across 3 zones in westus2?" or "give me a 4-core Arm machine type that is not
restricted for this subscription".

## Install

Prefer the prebuilt release binary over building from source:

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')                     # linux | darwin
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/') # amd64 | arm64
curl -fsSL -o skutool "https://github.com/omnistrate-community/azure-sku-tool/releases/latest/download/skutool-${OS}-${ARCH}"
chmod +x skutool
```

Verify against `checksums.txt` from the same release if integrity matters. To build from source
(Go toolchain required): `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o skutool ./`.

## Authentication

The tool authenticates **only** with Azure workload identity federation
(`azidentity.WorkloadIdentityCredential`). It does not use `az login`, client secrets,
managed identity, or environment-variable credentials. You need:

| Flag | Value |
|---|---|
| `--tenant-id` | Microsoft Entra tenant ID |
| `--client-id` | Client ID of the app registration / user-assigned identity that has a federated credential |
| `--subscription-id` | Subscription to evaluate. Restrictions are per subscription, so this matters. |
| `--identity-token-path` | Path to an OIDC token file that the federated credential trusts |

`--identity-token-path` defaults to `/var/run/secrets/eks.amazonaws.com/serviceaccount/token`
(the EKS service-account token). Common alternatives:

- AKS workload identity: `/var/run/secrets/azure/tokens/azure-identity-token` (also exported as `$AZURE_FEDERATED_TOKEN_FILE`; the pod usually exports `$AZURE_CLIENT_ID` and `$AZURE_TENANT_ID` too)
- Any other OIDC issuer: whatever file holds a current token for that issuer

The identity needs read access to `Microsoft.Compute/skus` on the subscription (the built-in
`Reader` role is sufficient).

If no federated token is available, do not try to make `skutool` work with other credentials.
Use the fallback in [Without skutool](#without-skutool).

## Running

```bash
./skutool \
  --tenant-id "$TENANT_ID" \
  --client-id "$CLIENT_ID" \
  --subscription-id "$SUBSCRIPTION_ID" \
  --identity-token-path "$TOKEN_FILE" \
  --sku-filter "location eq 'westus2'" \
  --restriction-exclude-filter NotAvailableForSubscription \
  --min-availability-zones 3 \
  > skus.json 2>&1
```

### Flags

| Flag | Default | Notes |
|---|---|---|
| `--sku-filter` | none | OData filter sent to the Azure API. The API only supports `location eq '<region>'`; keep the single quotes around the region. Always set it: without it the tool pages through every region, which is slow and large. |
| `--region-filter` | none | Client-side region match (case-insensitive). Redundant if `--sku-filter` is set. |
| `--restriction-exclude-filter` | none | Drops SKUs that have a restriction with this exact `reasonCode`. Values: `NotAvailableForSubscription`, `QuotaId`. Takes one value. |
| `--min-availability-zones` | `3` | Drops SKUs offered in fewer zones. Use `0` to include non-zonal SKUs and regions without zones. |

### Output

- **Results go to stderr**, not stdout. Always redirect with `2>&1` (or `2> file`).
- The output has one JSON object per line (zerolog). Each SKU line looks like this:
  ```json
  {"level":"info","instanceType":"Standard_D4ps_v6","restrictions":[],"locationInfo":{"Name":"westus2","AZs":["1","2","3"]},"time":"...","message":"Instance Type Availability Zones"}
  ```
- Line order is random. Sort before comparing runs.
- The output includes **non-VM SKUs**. Filter them out:
  - Disk SKUs: `Standard_LRS`, `StandardSSD_LRS`, `Premium_LRS`, `PremiumV2_LRS`, `UltraSSD_LRS`, `*_ZRS`
  - Dedicated-host SKUs without the `Standard_` prefix, such as `DSv5-Type1` and `MSmv2-Type1`
- `restrictions` uses Azure's `ResourceSkuRestrictions` shape:
  - `reasonCode` is `NotAvailableForSubscription` or `QuotaId`.
  - `type` is `Location` (blocked in the whole region) or `Zone`.
  - `restrictionInfo.zones` lists the **blocked** zones. A SKU can show 3 AZs but still be blocked in some of them.
- The tool reports only names, regions, zones and restrictions. It does **not** report vCPU,
  memory or price. Infer cores from the size name (see below) or check Azure documentation.
- Exit codes:
  - Missing required flags exit non-zero with a `fatal` log line.
  - Auth or API failures **panic** with a Go stack trace, and the useful error is near the top.
    Common causes are an expired token file, a wrong client or tenant ID, a missing federated
    credential, or a missing Reader role.

## Recipes (jq)

All eligible VM sizes, sorted:

```bash
jq -r 'select(.instanceType? // "" | test("^Standard_") and (test("_(LRS|ZRS)$") | not))
       | select((.restrictions | length) == 0) | .instanceType' skus.json | sort -V
```

Every eligible size in one series, e.g. the Arm Dpsv6 family (`Standard_D<n>p[l][d]s_v6`):

```bash
jq -r '.instanceType? // empty' skus.json | grep -E '^Standard_D[0-9]+pl?d?s_v6$' | sort -V
```

A specific core count (match the number exactly, so `D4` does not also match `D48`):

```bash
jq -r '.instanceType? // empty' skus.json | grep -E '^Standard_D4pl?d?s_v6$'
```

Why a size is missing: rerun **without** `--restriction-exclude-filter` and with
`--min-availability-zones 0`, then inspect it:

```bash
jq -c 'select(.instanceType == "Standard_D4s_v5") | {AZs: .locationInfo.AZs, restrictions}' skus-all.json
```

- If there is no line at all, the size is not offered in that region.
- If the line has `restrictions`, the subscription is blocked. Unblocking it needs an Azure
  support or quota request.
- If the size has fewer AZs than you need, it is not zonal enough.

Count by reason:

```bash
jq -r 'select(.instanceType? // "" | startswith("Standard_"))
       | [(.restrictions | map(.reasonCode) | unique | join(",") | if . == "" then "none" else . end),
          (.locationInfo.AZs | length)] | @tsv' skus-all.json | sort | uniq -c
```

## Reading Azure size names

Pattern: `Standard_<Family><vCPUs>[-<activeVCPUs>]<features>_v<generation>`, e.g. `Standard_D4pds_v6`.

- **Family:**
  - `B`: burstable
  - `D`: general purpose
  - `E`: memory optimized
  - `F`: compute optimized
  - `FX`: high-frequency compute
  - `L`: storage optimized
  - `M`: very large memory
  - `NC`/`ND`/`NV`: GPU
  - `DC`/`EC`: confidential computing
- **Number:** the vCPU count. With a hyphen (`F16-8amds_v7`), the second number is the active
  (constrained) vCPU count and the first is the size it is based on.
- **Feature letters:**
  - `a`: AMD
  - `p`: Arm64 (Ampere/Cobalt); workloads need arm64 images
  - `d`: local temp disk
  - `l`: low memory per vCPU
  - `m`: memory intensive
  - `s`: premium storage capable
  - `i`: isolated
  - `b`: higher remote-storage throughput
  - `t`: tiny memory
  - No `a` or `p` means Intel x86.
- **Version:** a newer `_vN` is usually better value. Older generations are often restricted on
  new subscriptions, so when an older size is blocked, suggest the same shape on the newest
  eligible generation.

When recommending a size, state its architecture (x86 or Arm), whether it has a local disk,
and the zones it is available in from the output.

## Without skutool

If no federated token is available but the Azure CLI is logged in, this command gives the same
data:

```bash
az vm list-skus --location westus2 --resource-type virtualMachines --all -o json \
  | jq -r '.[] | select((.locationInfo[0].zones | length) >= 3)
                | select([.restrictions[] | select(.reasonCode == "NotAvailableForSubscription")] | length == 0)
                | .name' | sort -V
```

`--all` matters: without it, the CLI hides restricted SKUs instead of showing their
restrictions. Unlike `skutool`, this output also includes vCPU and memory in `.capabilities`.
