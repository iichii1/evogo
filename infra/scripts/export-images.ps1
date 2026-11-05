param(
  [Parameter(Mandatory = $true)]
  [string]$ImagePrefix,

  [string]$Tag = "latest",
  [string]$OutputDir = "dist\images",

  # Versao gravada no binario; por padrao, o arquivo VERSION (veja publish-images.ps1).
  [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$bundleDir = Join-Path $repoRoot $OutputDir
$bundlePath = Join-Path $bundleDir "evogo-$Tag.tar"

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

function Build-Image {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name
  )

  $image = Resolve-ImageName -Prefix $ImagePrefix -Service $Name -ImageTag $Tag
  $dockerArgs = @("build", "-f", "Dockerfile", "--build-arg", "VERSION=$Version", "-t", $image, ".")

  Write-Host "==> Building $image"
  & docker @dockerArgs
  if ($LASTEXITCODE -ne 0) { throw "Falha no build de $image" }

  return $image
}

Push-Location $repoRoot
try {
  Test-ImagePrefixFormat -Prefix $ImagePrefix
  if (-not $Version) { $Version = (Get-Content -Raw (Join-Path $repoRoot "VERSION")).Trim() }
  if (!(Test-Path $bundleDir)) {
    New-Item -ItemType Directory -Path $bundleDir | Out-Null
  }

  $images = @()
  $images += Build-Image -Name "app"

  Write-Host "==> Exporting bundle to $bundlePath"
  & docker save -o $bundlePath $images
  if ($LASTEXITCODE -ne 0) { throw "Falha ao exportar bundle" }
}
finally {
  Pop-Location
}
