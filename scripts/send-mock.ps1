param(
    [string]$URL = 'http://localhost:8080/webhooks/github',
    [string]$Repository = 'example/project',
    [string]$DeliveryID = 'local-mock-001',
    [string]$Label = 'agent:ready'
)
$ErrorActionPreference = 'Stop'
if (-not $env:GITHUB_WEBHOOK_SECRET) { throw 'Set GITHUB_WEBHOOK_SECRET to the service secret first.' }
$payload = @{
    action = 'labeled'
    label = @{ name = $Label }
    repository = @{ full_name = $Repository }
    issue = @{ number = 42; state = 'open'; html_url = "https://github.com/$Repository/issues/42" }
} | ConvertTo-Json -Depth 4 -Compress
$body = [Text.Encoding]::UTF8.GetBytes($payload)
$hmac = [Security.Cryptography.HMACSHA256]::new([Text.Encoding]::UTF8.GetBytes($env:GITHUB_WEBHOOK_SECRET))
try { $signature = 'sha256=' + [Convert]::ToHexString($hmac.ComputeHash($body)).ToLowerInvariant() }
finally { $hmac.Dispose() }
$response = Invoke-WebRequest -Method Post -Uri $URL -ContentType 'application/json' -Body $body -Headers @{
    'X-Hub-Signature-256' = $signature
    'X-GitHub-Event' = 'issues'
    'X-GitHub-Delivery' = $DeliveryID
}
Write-Output "HTTP $($response.StatusCode) (202 = created, 200 = duplicate, 204 = ignored)"
