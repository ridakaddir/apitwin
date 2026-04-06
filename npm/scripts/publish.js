#!/usr/bin/env node

/**
 * publish.js — Automates the npm release of apitwin platform packages.
 *
 * This script:
 * 1. Downloads the Go binaries from a GitHub release
 * 2. Verifies checksums against the GoReleaser checksums.txt
 * 3. Extracts each binary into the correct platform package's bin/ directory
 * 4. Updates the version in all package.json files
 * 5. Publishes the platform packages first, then the main package
 *
 * Usage:
 *   node npm/scripts/publish.js <version> [--dry-run]
 *
 * Examples:
 *   node npm/scripts/publish.js 0.2.0
 *   node npm/scripts/publish.js 0.2.0 --dry-run
 *
 * Environment:
 *   GITHUB_TOKEN — optional, for authenticated GitHub API requests
 *   NPM_TOKEN    — required for publishing (or use .npmrc)
 */

"use strict";

const { execSync } = require("child_process");
const crypto = require("crypto");
const fs = require("fs");
const path = require("path");
const https = require("https");
const { pipeline } = require("stream/promises");

// ─── Configuration ──────────────────────────────────────────────────────────

const REPO = "ridakaddir/apitwin";
const NPM_DIR = path.resolve(__dirname, "..");
const MAX_REDIRECTS = 5;

/**
 * Maps GoReleaser archive names to npm platform package directories.
 * Key format: the archive filename suffix (without extension).
 * Value: { dir, binary, archiveType }
 */
const PLATFORM_MAP = {
  darwin_arm64: {
    dir: "darwin-arm64",
    binary: "apitwin",
    archiveType: "tar.gz",
  },
  darwin_amd64: {
    dir: "darwin-x64",
    binary: "apitwin",
    archiveType: "tar.gz",
  },
  linux_amd64: {
    dir: "linux-x64",
    binary: "apitwin",
    archiveType: "tar.gz",
  },
  linux_arm64: {
    dir: "linux-arm64",
    binary: "apitwin",
    archiveType: "tar.gz",
  },
  windows_amd64: {
    dir: "win32-x64",
    binary: "apitwin.exe",
    archiveType: "zip",
  },
  windows_arm64: {
    dir: "win32-arm64",
    binary: "apitwin.exe",
    archiveType: "zip",
  },
};

// All package directories (platform + main)
const PLATFORM_DIRS = Object.values(PLATFORM_MAP).map((p) => p.dir);
const ALL_PACKAGE_DIRS = [...PLATFORM_DIRS, "apitwin"];

// ─── Helpers ────────────────────────────────────────────────────────────────

function log(msg) {
  console.log(`\x1b[36m[publish]\x1b[0m ${msg}`);
}

function error(msg) {
  console.error(`\x1b[31m[publish]\x1b[0m ${msg}`);
  process.exit(1);
}

function exec(cmd, opts = {}) {
  log(`$ ${cmd}`);
  return execSync(cmd, { stdio: "inherit", ...opts });
}

/**
 * Computes the SHA256 hex digest of a file.
 */
function sha256(filePath) {
  const data = fs.readFileSync(filePath);
  return crypto.createHash("sha256").update(data).digest("hex");
}

/**
 * Sends an HTTPS GET and follows redirects, returning the final response stream.
 * - Caps redirect hops at MAX_REDIRECTS
 * - Refuses non-HTTPS redirects to prevent downgrade attacks
 * - Intentionally does NOT forward Authorization header on redirects
 */
function fetchWithRedirects(url) {
  return new Promise((resolve, reject) => {
    let redirectCount = 0;

    const handleResponse = (res) => {
      if (
        res.statusCode >= 300 &&
        res.statusCode < 400 &&
        res.headers.location
      ) {
        if (++redirectCount > MAX_REDIRECTS) {
          res.resume();
          reject(new Error(`Too many redirects (>${MAX_REDIRECTS}) for ${url}`));
          return;
        }

        const redirectUrl = res.headers.location;

        if (!redirectUrl.startsWith("https://")) {
          res.resume();
          reject(
            new Error(
              `Refusing to follow non-HTTPS redirect: ${redirectUrl}`
            )
          );
          return;
        }

        // Intentionally NOT forwarding Authorization header to redirected host
        res.resume();
        https
          .get(
            redirectUrl,
            { headers: { "User-Agent": "apitwin-npm-publish" } },
            handleResponse
          )
          .on("error", reject);
        return;
      }

      if (res.statusCode !== 200) {
        res.resume();
        reject(new Error(`Download failed: HTTP ${res.statusCode} for ${url}`));
        return;
      }

      resolve(res);
    };

    const headers = { "User-Agent": "apitwin-npm-publish" };
    if (process.env.GITHUB_TOKEN) {
      headers.Authorization = `token ${process.env.GITHUB_TOKEN}`;
    }

    https.get(url, { headers }, handleResponse).on("error", reject);
  });
}

/**
 * Downloads a file from a URL, following redirects (HTTPS only).
 *
 * Uses stream.pipeline to ensure the file handle is always closed,
 * response abort/error events are handled, and partially-written
 * files are cleaned up on failure.
 */
async function download(url, dest) {
  const res = await fetchWithRedirects(url);

  try {
    await pipeline(res, fs.createWriteStream(dest));
  } catch (err) {
    // Clean up partially-written file on failure
    try {
      fs.unlinkSync(dest);
    } catch {}
    throw err;
  }
}

/**
 * Parses GoReleaser checksums.txt into a Map<filename, sha256>.
 * Format: "<sha256>  <filename>\n"
 */
function parseChecksums(checksumFilePath) {
  const content = fs.readFileSync(checksumFilePath, "utf8");
  const checksums = new Map();
  for (const line of content.trim().split("\n")) {
    const parts = line.trim().split(/\s+/);
    if (parts.length === 2) {
      checksums.set(parts[1], parts[0]);
    }
  }
  return checksums;
}

/**
 * Checks if a package version already exists on the npm registry.
 */
function isPublished(packageName, version) {
  try {
    execSync(`npm view ${packageName}@${version} version`, {
      stdio: "pipe",
    });
    return true;
  } catch {
    return false;
  }
}

// ─── Main ───────────────────────────────────────────────────────────────────

async function main() {
  const args = process.argv.slice(2);
  const dryRun = args.includes("--dry-run");
  const version = args.find((a) => !a.startsWith("-"));

  if (!version) {
    error(
      "Usage: node npm/scripts/publish.js <version> [--dry-run]\n" +
        "  Example: node npm/scripts/publish.js 0.2.0"
    );
  }

  // Validate version format
  if (!/^\d+\.\d+\.\d+(-[\w.]+)?$/.test(version)) {
    error(
      `Invalid version format: ${version}\n` +
        `  Expected: X.Y.Z or X.Y.Z-beta.N`
    );
  }

  log(`Publishing apitwin v${version}${dryRun ? " (DRY RUN)" : ""}`);

  // ── Step 1: Update version in all package.json files ──────────────────

  log("Updating versions in all package.json files...");

  for (const dir of ALL_PACKAGE_DIRS) {
    const pkgPath = path.join(NPM_DIR, dir, "package.json");
    const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf8"));
    pkg.version = version;

    // Also update optionalDependencies versions in the main package
    if (dir === "apitwin" && pkg.optionalDependencies) {
      for (const dep of Object.keys(pkg.optionalDependencies)) {
        pkg.optionalDependencies[dep] = version;
      }
    }

    fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");
    log(`  Updated ${dir}/package.json -> v${version}`);
  }

  // ── Step 2: Download and extract binaries ─────────────────────────────

  const tmpDir = path.join(NPM_DIR, ".tmp");
  fs.mkdirSync(tmpDir, { recursive: true });

  try {
    // Download checksums file first for integrity verification
    const checksumsUrl = `https://github.com/${REPO}/releases/download/v${version}/checksums.txt`;
    const checksumsPath = path.join(tmpDir, "checksums.txt");

    log("Downloading checksums.txt...");
    try {
      await download(checksumsUrl, checksumsPath);
    } catch (e) {
      error(`Failed to download checksums: ${e.message}`);
    }

    const checksums = parseChecksums(checksumsPath);
    log(`  Loaded ${checksums.size} checksums`);

    log("Downloading binaries from GitHub release...");

    for (const [platform, config] of Object.entries(PLATFORM_MAP)) {
      const ext = config.archiveType === "zip" ? "zip" : "tar.gz";
      const archiveName = `apitwin_${platform}.${ext}`;
      const archiveUrl = `https://github.com/${REPO}/releases/download/v${version}/${archiveName}`;
      const archivePath = path.join(tmpDir, archiveName);
      const binDir = path.join(NPM_DIR, config.dir, "bin");

      log(`  Downloading ${archiveName}...`);

      try {
        await download(archiveUrl, archivePath);
      } catch (e) {
        error(`Failed to download ${archiveUrl}: ${e.message}`);
      }

      // Verify checksum
      const expectedHash = checksums.get(archiveName);
      if (!expectedHash) {
        error(
          `No checksum found for ${archiveName} in checksums.txt. ` +
            `Available: ${[...checksums.keys()].join(", ")}`
        );
      }

      const actualHash = sha256(archivePath);
      if (actualHash !== expectedHash) {
        error(
          `Checksum mismatch for ${archiveName}!\n` +
            `  Expected: ${expectedHash}\n` +
            `  Actual:   ${actualHash}\n` +
            `  This may indicate a corrupted download or tampered release.`
        );
      }
      log(`  Checksum verified for ${archiveName}`);

      // Extract the binary
      log(`  Extracting ${config.binary} -> ${config.dir}/bin/`);

      // Clean any existing binary or .gitkeep
      const binaryDest = path.join(binDir, config.binary);
      const gitkeep = path.join(binDir, ".gitkeep");
      if (fs.existsSync(binaryDest)) fs.unlinkSync(binaryDest);
      if (fs.existsSync(gitkeep)) fs.unlinkSync(gitkeep);

      if (config.archiveType === "tar.gz") {
        exec(
          `tar -xzf "${archivePath}" -C "${binDir}" --no-same-owner apitwin`,
          { stdio: "pipe" }
        );
      } else {
        // Windows zip
        exec(`unzip -o "${archivePath}" apitwin.exe -d "${binDir}"`, {
          stdio: "pipe",
        });
      }

      // Verify the binary was actually extracted
      if (!fs.existsSync(binaryDest)) {
        error(
          `Binary not found after extraction: ${binaryDest}\n` +
            `  The archive structure may have changed. Check the GoReleaser config.`
        );
      }

      // Ensure the binary is executable (non-Windows)
      if (config.binary === "apitwin") {
        fs.chmodSync(binaryDest, 0o755);
      }

      log(`  ${config.dir}/bin/${config.binary} ready`);
    }

    // ── Step 3: Publish platform packages ─────────────────────────────────

    log("Publishing platform packages...");

    const npmPublishFlags = [
      "--access public", // Scoped packages need --access public
      "--provenance",    // Supply-chain security: links package to source
      dryRun ? "--dry-run" : "",
    ]
      .filter(Boolean)
      .join(" ");

    for (const dir of PLATFORM_DIRS) {
      const pkgName = `@ridakaddir/apitwin-${dir}`;

      // Skip if already published (prevents partial-publish failures)
      if (!dryRun && isPublished(pkgName, version)) {
        log(`  ${pkgName}@${version} already published, skipping`);
        continue;
      }

      const pkgDir = path.join(NPM_DIR, dir);
      log(`  Publishing ${pkgName}...`);
      exec(`npm publish ${npmPublishFlags}`, { cwd: pkgDir });
    }

    // Brief pause to allow npm registry propagation before publishing the main
    // package that depends on the platform packages via optionalDependencies.
    if (!dryRun) {
      log("Waiting 30s for npm registry propagation...");
      await new Promise((resolve) => setTimeout(resolve, 30_000));
    }

    // ── Step 4: Publish main package ──────────────────────────────────────

    const mainPkgName = "@ridakaddir/apitwin";
    if (!dryRun && isPublished(mainPkgName, version)) {
      log(`  ${mainPkgName}@${version} already published, skipping`);
    } else {
      log("Publishing main package (@ridakaddir/apitwin)...");
      const mainPkgDir = path.join(NPM_DIR, "apitwin");
      const mainPublishFlags = [
        "--access public", // Scoped packages need --access public
        "--provenance",
        dryRun ? "--dry-run" : "",
      ]
        .filter(Boolean)
        .join(" ");
      exec(`npm publish ${mainPublishFlags}`, { cwd: mainPkgDir });
    }

    log(`\x1b[32mDone! Published apitwin v${version} to npm.\x1b[0m`);

    if (dryRun) {
      log("(This was a dry run — nothing was actually published.)");
    }
  } finally {
    // ── Cleanup — always runs, even on failure ──────────────────────────
    log("Cleaning up temporary files...");
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

main().catch((e) => {
  error(e.message);
});
