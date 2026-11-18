function Gw($path, $obj = @{}, $token = $null) {
  $headers = @{}
  if ($token) { $headers["Authorization"] = "Bearer $token" }
  try {
    Invoke-RestMethod -Method Post -Uri "http://localhost:8000$path" -Headers $headers `
      -ContentType "application/json" -Body ($obj | ConvertTo-Json)
  } catch {
    "HTTP $($_.Exception.Response.StatusCode.value__): $($_.ErrorDetails.Message)"
  }
}