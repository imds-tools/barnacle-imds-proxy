# Recipes

Minimal credential servers you can run locally alongside the extension. Each listens on a port, receives forwarded IMDS requests from Barnacle, reads container labels from the `x-container-labels` header, and responds in the format the cloud SDK expects.

Point the extension at `http://localhost:<port>` in the Settings tab.

---

## AWS

Handles credentials and region in one server. Reads `AWS_PROFILE` and `AWS_DEFAULT_REGION` labels from the requesting container. Both fall back to host environment values if not set. Requires the AWS CLI and `jq`.

Label your container:

```yaml
labels:
  - "imds-proxy.enabled=true"
  - "AWS_PROFILE=my-profile"
  - "AWS_DEFAULT_REGION=us-west-2"
```

zsh/bash:

```bash
#!/usr/bin/env bash
PORT=${1:-8080}
while true; do
  { read -r line
    PATH_REQ=$(echo "$line" | awk '{print $2}')
    while IFS= read -r h && [ "$h" != $'\r' ]; do
      [[ "${h,,}" == x-container-labels:* ]] && LABELS="${h#*: }"
    done
    PROFILE=$(echo "$LABELS" | jq -r '.AWS_PROFILE // "default"')
    REGION=$(echo "$LABELS" | jq -r '.AWS_DEFAULT_REGION // empty')
    REGION=${REGION:-${AWS_DEFAULT_REGION:-us-east-1}}
    if [[ "$PATH_REQ" == */placement/region ]]; then
      printf "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: ${#REGION}\r\nConnection: close\r\n\r\n$REGION"
    else
      CREDS=$(AWS_PROFILE=$PROFILE aws sts get-session-token --query Credentials --output json)
      BODY=$(echo "$CREDS" | jq -c '{Code:"Success",Type:"AWS-HMAC",
        AccessKeyId:.AccessKeyId,SecretAccessKey:.SecretAccessKey,
        Token:.SessionToken,Expiration:.Expiration}')
      printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${#BODY}\r\nConnection: close\r\n\r\n$BODY"
    fi
  } | nc -l -p $PORT -q 1
done
```

PowerShell:

```powershell
$port = 8080
$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://localhost:$port/")
$listener.Prefixes.Add("http://host.docker.internal:$port/")
$listener.Start()
Write-Host "AWS IMDS server listening on port $port"
while ($listener.IsListening) {
    $ctx = $listener.GetContext()
    $labels = $ctx.Request.Headers["x-container-labels"] | ConvertFrom-Json -AsHashtable
    $profile = if ($labels?["AWS_PROFILE"]) { $labels["AWS_PROFILE"] } else { "default" }
    $region  = if ($labels?["AWS_DEFAULT_REGION"]) { $labels["AWS_DEFAULT_REGION"] }
               elseif ($env:AWS_DEFAULT_REGION) { $env:AWS_DEFAULT_REGION }
               else { "us-east-1" }
    if ($ctx.Request.RawUrl -match "placement/region") {
        $bytes = [Text.Encoding]::UTF8.GetBytes($region)
        $ctx.Response.ContentType = "text/plain"
    } else {
        $creds = aws sts get-session-token --profile $profile --query Credentials --output json | ConvertFrom-Json
        $body  = @{ Code="Success"; Type="AWS-HMAC"; AccessKeyId=$creds.AccessKeyId
                    SecretAccessKey=$creds.SecretAccessKey; Token=$creds.SessionToken
                    Expiration=$creds.Expiration } | ConvertTo-Json -Compress
        $bytes = [Text.Encoding]::UTF8.GetBytes($body)
        $ctx.Response.ContentType = "application/json"
    }
    $ctx.Response.ContentLength64 = $bytes.Length
    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $ctx.Response.Close()
}
```

---

## Azure

Returns an access token for the resource requested by the container. Reads the `resource` query parameter from the IMDS request and passes it to the Azure CLI. The `AZURE_CLIENT_ID` label selects a specific managed identity; omit it to use the active `az` account. Requires the Azure CLI.

Label your container:

```yaml
labels:
  - "imds-proxy.enabled=true"
  - "AZURE_CLIENT_ID=<optional-managed-identity-client-id>"
```

zsh/bash:

```bash
#!/usr/bin/env bash
PORT=${1:-8080}
while true; do
  { read -r line
    QUERY=$(echo "$line" | awk '{print $2}' | grep -o 'resource=[^&]*' | cut -d= -f2-)
    RESOURCE=${QUERY:-https://management.azure.com/}
    while IFS= read -r h && [ "$h" != $'\r' ]; do
      [[ "${h,,}" == x-container-labels:* ]] && LABELS="${h#*: }"
    done
    CLIENT_ID=$(echo "$LABELS" | jq -r '.AZURE_CLIENT_ID // empty')
    TOKEN=$(az account get-access-token --resource "$RESOURCE" ${CLIENT_ID:+--client-id "$CLIENT_ID"} --output json)
    BODY=$(echo "$TOKEN" | jq -c '{access_token:.accessToken,expires_in:3599,token_type:"Bearer"}')
    printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${#BODY}\r\nConnection: close\r\n\r\n$BODY"
  } | nc -l -p $PORT -q 1
done
```

PowerShell:

```powershell
$port = 8080
$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://localhost:$port/")
$listener.Prefixes.Add("http://host.docker.internal:$port/")
$listener.Start()
Write-Host "Azure IMDS server listening on port $port"
while ($listener.IsListening) {
    $ctx = $listener.GetContext()
    $labels   = $ctx.Request.Headers["x-container-labels"] | ConvertFrom-Json -AsHashtable
    $resource = if ($ctx.Request.QueryString["resource"]) { $ctx.Request.QueryString["resource"] }
                else { "https://management.azure.com/" }
    $clientId = $labels?["AZURE_CLIENT_ID"]
    $args     = @("account", "get-access-token", "--resource", $resource, "--output", "json")
    if ($clientId) { $args += "--client-id"; $args += $clientId }
    $token = & az @args | ConvertFrom-Json
    $body  = @{ access_token=$token.accessToken; expires_in=3599; token_type="Bearer" } | ConvertTo-Json -Compress
    $bytes = [Text.Encoding]::UTF8.GetBytes($body)
    $ctx.Response.ContentType = "application/json"
    $ctx.Response.ContentLength64 = $bytes.Length
    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $ctx.Response.Close()
}
```

---

## GCP

Returns an access token for the service account named in the container's `GCP_SERVICE_ACCOUNT` label. Falls back to the active `gcloud` account if the label is not set. Requires the `gcloud` CLI.

Label your container:

```yaml
labels:
  - "imds-proxy.enabled=true"
  - "GCP_SERVICE_ACCOUNT=my-sa@my-project.iam.gserviceaccount.com"
```

zsh/bash:

```bash
#!/usr/bin/env bash
PORT=${1:-8080}
while true; do
  { read -r _req
    while IFS= read -r h && [ "$h" != $'\r' ]; do
      [[ "${h,,}" == x-container-labels:* ]] && LABELS="${h#*: }"
    done
    SA=$(echo "$LABELS" | jq -r '.GCP_SERVICE_ACCOUNT // empty')
    TOKEN=$(gcloud auth print-access-token ${SA:+--impersonate-service-account=$SA})
    EXPIRY=$(date -u -d "+3599 seconds" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -v+3599S +"%Y-%m-%dT%H:%M:%SZ")
    BODY="{\"access_token\":\"$TOKEN\",\"expires_in\":3599,\"token_type\":\"Bearer\"}"
    printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${#BODY}\r\nConnection: close\r\n\r\n$BODY"
  } | nc -l -p $PORT -q 1
done
```

PowerShell:

```powershell
$port = 8080
$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://localhost:$port/")
$listener.Prefixes.Add("http://host.docker.internal:$port/")
$listener.Start()
Write-Host "GCP IMDS server listening on port $port"
while ($listener.IsListening) {
    $ctx = $listener.GetContext()
    $labels = $ctx.Request.Headers["x-container-labels"] | ConvertFrom-Json -AsHashtable
    $sa    = $labels?["GCP_SERVICE_ACCOUNT"]
    $token = if ($sa) { gcloud auth print-access-token --impersonate-service-account=$sa }
             else { gcloud auth print-access-token }
    $body  = @{ access_token=$token.Trim(); expires_in=3599; token_type="Bearer" } | ConvertTo-Json -Compress
    $bytes = [Text.Encoding]::UTF8.GetBytes($body)
    $ctx.Response.ContentType = "application/json"
    $ctx.Response.ContentLength64 = $bytes.Length
    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $ctx.Response.Close()
}
```

---

## Alibaba Cloud

Add `100.100.100.200` in Settings to intercept Alibaba Cloud IMDS traffic.

Returns RAM role credentials. Reads the `ALIBABA_ROLE` label to select which RAM role to assume. Requires the `aliyun` CLI.

Label your container:

```yaml
labels:
  - "imds-proxy.enabled=true"
  - "ALIBABA_ROLE=my-ram-role"
  - "ALIBABA_ROLE_ARN=acs:ram::123456789:role/my-ram-role"
```

zsh/bash:

```bash
#!/usr/bin/env bash
PORT=${1:-8080}
while true; do
  { read -r _req
    while IFS= read -r h && [ "$h" != $'\r' ]; do
      [[ "${h,,}" == x-container-labels:* ]] && LABELS="${h#*: }"
    done
    ROLE_ARN=$(echo "$LABELS" | jq -r '.ALIBABA_ROLE_ARN // empty')
    CREDS=$(aliyun sts AssumeRole --RoleArn "$ROLE_ARN" --RoleSessionName barnacle-session --output json)
    BODY=$(echo "$CREDS" | jq -c '.Credentials | {Code:"Success",AccessKeyId:.AccessKeyId,
      AccessKeySecret:.AccessKeySecret,SecurityToken:.SecurityToken,Expiration:.Expiration}')
    printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${#BODY}\r\nConnection: close\r\n\r\n$BODY"
  } | nc -l -p $PORT -q 1
done
```

PowerShell:

```powershell
$port = 8080
$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://localhost:$port/")
$listener.Prefixes.Add("http://host.docker.internal:$port/")
$listener.Start()
Write-Host "Alibaba Cloud IMDS server listening on port $port"
while ($listener.IsListening) {
    $ctx     = $listener.GetContext()
    $labels  = $ctx.Request.Headers["x-container-labels"] | ConvertFrom-Json -AsHashtable
    $roleArn = $labels?["ALIBABA_ROLE_ARN"]
    $creds   = aliyun sts AssumeRole --RoleArn $roleArn --RoleSessionName barnacle-session --output json | ConvertFrom-Json
    $body    = @{ Code="Success"; AccessKeyId=$creds.Credentials.AccessKeyId
                  AccessKeySecret=$creds.Credentials.AccessKeySecret
                  SecurityToken=$creds.Credentials.SecurityToken
                  Expiration=$creds.Credentials.Expiration } | ConvertTo-Json -Compress
    $bytes = [Text.Encoding]::UTF8.GetBytes($body)
    $ctx.Response.ContentType = "application/json"
    $ctx.Response.ContentLength64 = $bytes.Length
    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $ctx.Response.Close()
}
```

---

## Tencent Cloud

Add `169.254.0.23` in Settings to intercept Tencent Cloud IMDS traffic.

Returns CAM role credentials. Reads the `TENCENT_ROLE` label to select which CAM role to use. Requires the `tccli` CLI.

Label your container:

```yaml
labels:
  - "imds-proxy.enabled=true"
  - "TENCENT_ROLE=my-cam-role"
```

zsh/bash:

```bash
#!/usr/bin/env bash
PORT=${1:-8080}
while true; do
  { read -r _req
    while IFS= read -r h && [ "$h" != $'\r' ]; do
      [[ "${h,,}" == x-container-labels:* ]] && LABELS="${h#*: }"
    done
    ROLE=$(echo "$LABELS" | jq -r '.TENCENT_ROLE // empty')
    CREDS=$(tccli sts AssumeRole --RoleArn "qcs::cam::uin/$(tccli sts GetCallerIdentity --output json | jq -r '.UserId'):roleName/$ROLE" --RoleSessionName barnacle-session --output json)
    BODY=$(echo "$CREDS" | jq -c '.Credentials | {Code:"Success",TmpSecretId:.TmpSecretId,
      TmpSecretKey:.TmpSecretKey,Token:.Token,ExpiredTime:(.ExpiredTime|tostring)}')
    printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${#BODY}\r\nConnection: close\r\n\r\n$BODY"
  } | nc -l -p $PORT -q 1
done
```

PowerShell:

```powershell
$port = 8080
$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://localhost:$port/")
$listener.Prefixes.Add("http://host.docker.internal:$port/")
$listener.Start()
Write-Host "Tencent Cloud IMDS server listening on port $port"
while ($listener.IsListening) {
    $ctx    = $listener.GetContext()
    $labels = $ctx.Request.Headers["x-container-labels"] | ConvertFrom-Json -AsHashtable
    $role   = $labels?["TENCENT_ROLE"]
    $uid    = tccli sts GetCallerIdentity --output json | ConvertFrom-Json | Select-Object -ExpandProperty UserId
    $arn    = "qcs::cam::uin/${uid}:roleName/$role"
    $creds  = tccli sts AssumeRole --RoleArn $arn --RoleSessionName barnacle-session --output json | ConvertFrom-Json
    $body   = @{ Code="Success"; TmpSecretId=$creds.Credentials.TmpSecretId
                 TmpSecretKey=$creds.Credentials.TmpSecretKey
                 Token=$creds.Credentials.Token
                 ExpiredTime=$creds.Credentials.ExpiredTime } | ConvertTo-Json -Compress
    $bytes = [Text.Encoding]::UTF8.GetBytes($body)
    $ctx.Response.ContentType = "application/json"
    $ctx.Response.ContentLength64 = $bytes.Length
    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $ctx.Response.Close()
}
```

---

## Multi-cloud (AWS + Azure)

Routes to the AWS or Azure handler based on a `CLOUD_PROVIDER` container label. Each container declares which provider it needs; one server handles both. Extend the pattern to add more providers.

Label your container:

```yaml
labels:
  - "imds-proxy.enabled=true"
  - "CLOUD_PROVIDER=aws"          # or "azure"
  - "AWS_PROFILE=my-profile"      # AWS only
  - "AWS_DEFAULT_REGION=us-west-2" # AWS only, optional
  - "AZURE_CLIENT_ID=<client-id>" # Azure only, optional
```

zsh/bash:

```bash
#!/usr/bin/env bash
PORT=${1:-8080}
while true; do
  { read -r line
    PATH_REQ=$(echo "$line" | awk '{print $2}')
    while IFS= read -r h && [ "$h" != $'\r' ]; do
      [[ "${h,,}" == x-container-labels:* ]] && LABELS="${h#*: }"
    done
    PROVIDER=$(echo "$LABELS" | jq -r '.CLOUD_PROVIDER // "aws"')
    CTYPE="application/json"
    STATUS="200 OK"
    BODY=""
    if [[ "$PATH_REQ" == */placement/region ]]; then
      REGION=$(echo "$LABELS" | jq -r '.AWS_DEFAULT_REGION // empty')
      BODY=${REGION:-${AWS_DEFAULT_REGION:-us-east-1}}
      CTYPE="text/plain"
    elif [[ "$PATH_REQ" == */iam/security-credentials* && "$PROVIDER" == "aws" ]]; then
      PROFILE=$(echo "$LABELS" | jq -r '.AWS_PROFILE // "default"')
      CREDS=$(AWS_PROFILE=$PROFILE aws sts get-session-token --query Credentials --output json)
      BODY=$(echo "$CREDS" | jq -c '{Code:"Success",Type:"AWS-HMAC",
        AccessKeyId:.AccessKeyId,SecretAccessKey:.SecretAccessKey,
        Token:.SessionToken,Expiration:.Expiration}')
    elif [[ "$PATH_REQ" == *metadata/identity/oauth2/token* && "$PROVIDER" == "azure" ]]; then
      QUERY=$(echo "$PATH_REQ" | grep -o 'resource=[^&]*' | cut -d= -f2-)
      RESOURCE=${QUERY:-https://management.azure.com/}
      CLIENT_ID=$(echo "$LABELS" | jq -r '.AZURE_CLIENT_ID // empty')
      TOKEN=$(az account get-access-token --resource "$RESOURCE" ${CLIENT_ID:+--client-id "$CLIENT_ID"} --output json)
      BODY=$(echo "$TOKEN" | jq -c '{access_token:.accessToken,expires_in:3599,token_type:"Bearer"}')
    else
      STATUS="404 Not Found"
    fi
    printf "HTTP/1.1 $STATUS\r\nContent-Type: $CTYPE\r\nContent-Length: ${#BODY}\r\nConnection: close\r\n\r\n$BODY"
  } | nc -l -p $PORT -q 1
done
```

PowerShell:

```powershell
$port = 8080
$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://localhost:$port/")
$listener.Prefixes.Add("http://host.docker.internal:$port/")
$listener.Start()
Write-Host "Multi-cloud IMDS server listening on port $port"
while ($listener.IsListening) {
    $ctx      = $listener.GetContext()
    $labels   = $ctx.Request.Headers["x-container-labels"] | ConvertFrom-Json -AsHashtable
    $provider = if ($labels?["CLOUD_PROVIDER"]) { $labels["CLOUD_PROVIDER"] } else { "aws" }
    if ($ctx.Request.RawUrl -match "placement/region") {
        $body = if ($labels?["AWS_DEFAULT_REGION"]) { $labels["AWS_DEFAULT_REGION"] }
                elseif ($env:AWS_DEFAULT_REGION) { $env:AWS_DEFAULT_REGION }
                else { "us-east-1" }
        $ctx.Response.ContentType = "text/plain"
    } elseif ($ctx.Request.RawUrl -match "iam/security-credentials" -and $provider -eq "aws") {
        $profile = if ($labels?["AWS_PROFILE"]) { $labels["AWS_PROFILE"] } else { "default" }
        $creds = aws sts get-session-token --profile $profile --query Credentials --output json | ConvertFrom-Json
        $body  = @{ Code="Success"; Type="AWS-HMAC"; AccessKeyId=$creds.AccessKeyId
                    SecretAccessKey=$creds.SecretAccessKey; Token=$creds.SessionToken
                    Expiration=$creds.Expiration } | ConvertTo-Json -Compress
        $ctx.Response.ContentType = "application/json"
    } elseif ($ctx.Request.RawUrl -match "metadata/identity/oauth2/token" -and $provider -eq "azure") {
        $resource = if ($ctx.Request.QueryString["resource"]) { $ctx.Request.QueryString["resource"] }
                    else { "https://management.azure.com/" }
        $clientId = $labels?["AZURE_CLIENT_ID"]
        $args     = @("account", "get-access-token", "--resource", $resource, "--output", "json")
        if ($clientId) { $args += "--client-id"; $args += $clientId }
        $token = & az @args | ConvertFrom-Json
        $body  = @{ access_token=$token.accessToken; expires_in=3599; token_type="Bearer" } | ConvertTo-Json -Compress
        $ctx.Response.ContentType = "application/json"
    } else {
        $ctx.Response.StatusCode = 404
        $ctx.Response.Close()
        continue
    }
    $bytes = [Text.Encoding]::UTF8.GetBytes($body)
    $ctx.Response.ContentLength64 = $bytes.Length
    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $ctx.Response.Close()
}
```

---

## Other providers

**DigitalOcean** - The DigitalOcean metadata service provides droplet info (hostname, region, tags) but does not serve credentials. Use the DigitalOcean API directly with a personal access token.

**IBM Cloud** - IBM Cloud VPC metadata uses a two-step token exchange that is not easily served by a simple script. Use the [imds-server](https://github.com/imdsutil/imds-server) project for full IBM Cloud support.

**Oracle Cloud** - Oracle instance principal authentication is certificate-based and not scriptable in this way. Use the [imds-server](https://github.com/imdsutil/imds-server) project for full Oracle Cloud support.

**Salesforce Hyperforce** - Hyperforce runs on top of AWS, GCP, and Azure. Use the recipe for whichever underlying cloud your Hyperforce environment is on.

---

## Going further

These recipes handle the most common paths. A production-grade IMDS server handles the full path surface - instance identity, token endpoints, IMDSv2, and more - with proper routing, caching, and error handling. The [imds-server](https://github.com/imdsutil/imds-server) project is built for exactly this.
