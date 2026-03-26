#!/usr/bin/env node
'use strict'

const { spawnSync } = require('child_process')
const path = require('path')
const os = require('os')

function getBinaryPath() {
  const platform = os.platform() // 'linux' | 'darwin' | 'win32'
  const arch = os.arch()         // 'x64' | 'arm64'

  // Map Node arch names to our package arch names
  const archMap = { x64: 'x64', arm64: 'arm64' }
  const mappedArch = archMap[arch]

  if (!mappedArch) {
    fatal(`unsupported architecture: ${arch}`)
  }

  const pkgName = `@iamdecatalyst/hummingbird-${platform}-${mappedArch}`
  const binaryName = platform === 'win32' ? 'hummingbird.exe' : 'hummingbird'

  try {
    const pkgDir = path.dirname(require.resolve(`${pkgName}/package.json`))
    return path.join(pkgDir, 'bin', binaryName)
  } catch {
    fatal(
      `no binary found for ${platform}/${arch}\n` +
      `  expected package: ${pkgName}\n` +
      `  install manually: https://github.com/iamdecatalyst/hummingbird/releases`
    )
  }
}

function fatal(msg) {
  process.stderr.write(`\nhummingbird: ${msg}\n\n`)
  process.exit(1)
}

const bin = getBinaryPath()
const result = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' })

if (result.error) {
  fatal(`failed to run binary: ${result.error.message}`)
}

process.exit(result.status ?? 1)
