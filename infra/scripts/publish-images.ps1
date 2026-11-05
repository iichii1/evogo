# Build e push da imagem da EvoGo para o registry local (infra/README.md).
# Uso: .\infra\scripts\publish-images.ps1 -ImagePrefix "192.168.100.30:5000/evogo" -Tag "0.7.2-wf.1"
param(
  [Parameter(Mandatory = $true)]
  [string]$ImagePrefix,

  [string]$Tag = "latest",

  # Versao gravada no binario (-X main.version). Por padrao, o conteudo do arquivo VERSION.
  # O servidor manda essa versao ao servico de licenca no heartbeat: mantenha a versao da base
  # (ex.: 0.7.2) e diferencie a imagem so pela tag (ex.: 0.7.2-wf.1).
  [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")

function Test-ImagePrefixFormat {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Prefix
  )

  if ($Prefix -match '^registry\.(\d{1,3}(?:\.\d{1,3}){3}:\d+)(?:/.*)?$') {
    throw "Prefixo de imagem invalido: '$Prefix'. Use '192.168.100.30:5000/evogo' e nao 'registry.192.168.100.30:5000'."
  }
}

function Resolve-ImageName {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Prefix,
    [Parameter(Mandatory = $true)]
    [string]$Service,
    [Parameter(Mandatory = $true)]
    [string]$ImageTag
  )

  return "$Prefix/$Service`:$ImageTag"
}

function Test-TransientDockerPushError {
  param(
    [Parameter()]
    [AllowEmptyCollection()]
    [object[]]$Output
  )

  $patterns = @(
    '(?i)\bEOF\b',
    '(?i)connection reset',
    '(?i)broken pipe',
    '(?i)i/o timeout',
    '(?i)\btimeout\b',
    '(?i)temporarily unavailable',
    '(?i)TLS handshake timeout',
    '(?i)network is unreachable',
    '(?i)connection refused',
    '(?i)unexpected eof'
  )

  foreach ($line in @($Output)) {
    if ($null -eq $line) { continue }
    $text = $line.ToString()
    foreach ($pattern in $patterns) {
      if ($text -match $pattern) {
        return $true
      }
    }
  }

  return $false
}

function Invoke-DockerPushWithRetry {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Image,

    [Parameter()]
    [int]$MaxAttempts = 3,

    [Parameter()]
    [int]$InitialDelaySeconds = 5
  )

  $attempt = 1
  $delaySeconds = $InitialDelaySeconds

  while ($true) {
    Write-Host "==> Pushing $Image (attempt $attempt/$MaxAttempts)"
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
      $output = & docker push $Image 2>&1
      $exitCode = $LASTEXITCODE
    }
    finally {
      $ErrorActionPreference = $previousErrorActionPreference
    }

    foreach ($line in @($output)) {
      if ($null -ne $line) {
        Write-Host $line
      }
    }

    if ($exitCode -eq 0) {
      return
    }

    $shouldRetry = $attempt -lt $MaxAttempts -and (Test-TransientDockerPushError -Output $output)
    if (-not $shouldRetry) {
      throw "Falha no push de $Image"
    }

    Write-Warning "Erro de conexao detectado ao fazer push de $Image. Tentando novamente em $delaySeconds segundos..."
    Start-Sleep -Seconds $delaySeconds
    $attempt++
    $delaySeconds = [Math]::Min($delaySeconds * 2, 30)
  }
}

function Build-And-Push {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string]$BuildVersion
  )

  $image = Resolve-ImageName -Prefix $ImagePrefix -Service $Name -ImageTag $Tag
  $dockerArgs = @("build", "-f", "Dockerfile", "--build-arg", "VERSION=$BuildVersion", "-t", $image, ".")

  Write-Host "==> Building $image (versao do binario: $BuildVersion)"
  & docker @dockerArgs
  if ($LASTEXITCODE -ne 0) { throw "Falha no build de $image" }

  Invoke-DockerPushWithRetry -Image $image

  # O digest e o endereco imutavel da imagem: serve para fixar o stack (image: ...@sha256:...).
  $digest = (& docker image inspect $image --format "{{index .RepoDigests 0}}" 2>$null)
  if ($LASTEXITCODE -eq 0 -and $digest) {
    Write-Host "==> Imagem publicada: $image"
    Write-Host "==> Com digest:       $digest"
  }
}

Push-Location $repoRoot
try {
  Test-ImagePrefixFormat -Prefix $ImagePrefix

  if (-not $Version) {
    $versionFile = Join-Path $repoRoot "VERSION"
    if (-not (Test-Path $versionFile)) { throw "Informe -Version: o arquivo VERSION nao existe." }
    $Version = (Get-Content -Raw $versionFile).Trim()
  }
  if ($Version -notmatch '^[0-9A-Za-z][0-9A-Za-z._-]*$') {
    throw "Versao invalida: '$Version'."
  }

  Build-And-Push -Name "app" -BuildVersion $Version
}
finally {
  Pop-Location
}
