// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package setup

import "fmt"

// Windows is the binary the PowerShell script runs: limenctl for Docker Desktop on Windows.
const Windows = "limenctl-windows-amd64.exe"

// PowerShell is the setup script for Windows from the limen at base: it fetches limenctl, checks
// it against sum (its SHA-256), runs it with the arguments the script was given (plan when none;
// --limen base added to the commands that take it), and removes it. It runs in a script block of
// its own, so iex leaves no variables in the caller's session, and it never exits the caller's
// shell.
func PowerShell(base, sum string) []byte {
	return []byte(fmt.Sprintf(`# limen setup: runs limenctl from the limen at %[1]s on Windows without installing it.
#   irm %[1]s/setup.ps1 | iex
#   & ([scriptblock]::Create((irm %[1]s/setup.ps1))) configure --yes
# limen: https://github.com/nauarch/limen, Copyright 2026 darki73, Apache-2.0.
& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'
  $limen = '%[1]s'
  $dir = Join-Path ([IO.Path]::GetTempPath()) ('limen-' + [guid]::NewGuid())
  New-Item -ItemType Directory -Path $dir | Out-Null
  try {
    $exe = Join-Path $dir 'limenctl.exe'
    Invoke-WebRequest -UseBasicParsing -Uri "$limen/get/%[2]s" -OutFile $exe
    if ((Get-FileHash -Algorithm SHA256 -Path $exe).Hash -ne '%[3]s') {
      throw 'limen setup: the downloaded limenctl does not match its checksum'
    }
    $run = @($args)
    if ($run.Count -eq 0) { $run = @('plan') }
    if ($run[0] -in 'plan', 'configure', 'migrate', 'verify') { $run = @($run[0], '--limen', $limen) + @($run | Select-Object -Skip 1) }
    & $exe @run
  } finally {
    Remove-Item -Recurse -Force -Path $dir -ErrorAction SilentlyContinue
  }
} @args
`, base, Windows, sum))
}
