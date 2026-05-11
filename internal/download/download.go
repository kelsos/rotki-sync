package download

import (
	"archive/zip"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/kelsos/rotki-sync/internal/logger"
)

const (
	// GitHubAPIURL is the URL for the GitHub API to get the latest release
	GitHubAPIURL = "https://api.github.com/repos/rotki/rotki/releases/latest"
	// BinDir is the directory where the binary will be installed
	BinDir  = "bin"
	Darwin  = "darwin"
	Linux   = "linux"
	Windows = "windows"
	// InstallDirName is the folder inside BinDir that holds the extracted onedir bundle
	InstallDirName = "rotki-core"
)

// GithubAsset represents an asset in a GitHub release
type GithubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int    `json:"size"`
}

// GithubRelease represents a GitHub release
type GithubRelease struct {
	Name    string        `json:"name"`
	TagName string        `json:"tag_name"`
	Assets  []GithubAsset `json:"assets"`
}

// ensureBinDir ensures that the bin directory exists
func ensureBinDir() error {
	if _, err := os.Stat(BinDir); os.IsNotExist(err) {
		if err := os.MkdirAll(BinDir, 0755); err != nil {
			return fmt.Errorf("failed to create bin directory: %w", err)
		}
		logger.Info("Created bin directory at %s", BinDir)
	}
	return nil
}

// getAssetRegexPattern returns a regex pattern for the binary file based on the platform and architecture.
// As of rotki-core PyInstaller onedir, all platforms ship a .zip bundle.
func getAssetRegexPattern() *regexp.Regexp {
	var pattern string
	switch runtime.GOOS {
	case Darwin:
		if runtime.GOARCH == "arm64" {
			pattern = `rotki-core-(\d+\.\d+\.\d+)-macos-arm64\.zip$`
		} else {
			pattern = `rotki-core-(\d+\.\d+\.\d+)-macos-x64\.zip$`
		}
	case Linux:
		pattern = `rotki-core-(\d+\.\d+\.\d+)-linux\.zip$`
	case Windows:
		pattern = `rotki-core-(\d+\.\d+\.\d+)-windows\.zip$`
	default:
		logger.Fatal("Unsupported platform: %s", runtime.GOOS)
	}
	return regexp.MustCompile(pattern)
}

// getChecksumRegexPattern returns a regex pattern for the checksum file based on the binary file name
func getChecksumRegexPattern(binaryFileName string) *regexp.Regexp {
	escapedBinaryFileName := regexp.QuoteMeta(binaryFileName)
	pattern := fmt.Sprintf("^%s\\.sha512$", escapedBinaryFileName)
	return regexp.MustCompile(pattern)
}

// executableName returns the stable name of the rotki-core executable inside the install folder.
func executableName() string {
	if runtime.GOOS == Windows {
		return "rotki-core.exe"
	}
	return "rotki-core"
}

// InstalledBinaryPath returns the path to the installed rotki-core executable.
func InstalledBinaryPath() string {
	return filepath.Join(BinDir, InstallDirName, executableName())
}

// downloadFile downloads a file from a URL to a destination path
func downloadFile(downloadUrl, dest string) error {
	parsedURL, err := url.Parse(downloadUrl)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	if parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}

	if parsedURL.Host != "github.com" {
		return fmt.Errorf("unsupported URL host: %s", parsedURL.Host)
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", dest, err)
	}
	defer out.Close()

	// #nosec G107 - URL is constructed from trusted GitHub API responses, not user input
	resp, err := http.Get(downloadUrl)
	if err != nil {
		return fmt.Errorf("failed to download file from %s: %w", downloadUrl, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", dest, err)
	}

	return nil
}

// calculateChecksum calculates the SHA512 checksum of a file
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	hash := sha512.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// getLatestRelease gets the latest release information from GitHub
func getLatestRelease() (*GithubRelease, error) {
	req, err := http.NewRequest("GET", GitHubAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "rotki-core-downloader")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API request failed: %s", resp.Status)
	}

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	return &release, nil
}

// extractVersion extracts the version from a file name using a regex pattern
func extractVersion(fileName string, pattern *regexp.Regexp) (string, error) {
	match := pattern.FindStringSubmatch(fileName)
	if len(match) < 2 {
		return "", fmt.Errorf("could not extract version from file name: %s", fileName)
	}
	return match[1], nil
}

// parseChecksumFile parses a checksum file and returns the checksum
func parseChecksumFile(content string) string {
	content = strings.TrimSpace(content)
	parts := strings.Fields(content)
	if len(parts) > 0 {
		return parts[0]
	}
	return content
}

// verifyBinaryVersion verifies that the binary has the expected version
func verifyBinaryVersion(binaryPath, expectedVersion string) (bool, error) {
	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to execute version check: %w", err)
	}

	versionOutput := strings.TrimSpace(string(output))
	logger.Info("Binary version output: %s", versionOutput)

	re := regexp.MustCompile(`(\d+\.\d+\.\d+)`)
	match := re.FindStringSubmatch(versionOutput)
	if len(match) < 2 {
		return false, fmt.Errorf("could not extract version from output: %s", versionOutput)
	}

	actualVersion := match[1]
	logger.Info("Extracted version: %s, Expected version: %s", actualVersion, expectedVersion)

	return actualVersion == expectedVersion, nil
}

// prepareForDownload ensures the bin directory exists and gets the latest release info
func prepareForDownload() (*GithubRelease, error) {
	if err := ensureBinDir(); err != nil {
		return nil, err
	}

	release, err := getLatestRelease()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest release: %w", err)
	}

	return release, nil
}

// findReleaseAssets finds the appropriate asset and checksum file for the platform
func findReleaseAssets(release *GithubRelease) (*GithubAsset, *GithubAsset, string, error) {
	assetPattern := getAssetRegexPattern()
	var asset *GithubAsset
	for _, a := range release.Assets {
		if assetPattern.MatchString(a.Name) {
			asset = &a
			break
		}
	}

	if asset == nil {
		logger.Error("Available assets:")
		for _, a := range release.Assets {
			logger.Info("- %s", a.Name)
		}
		return nil, nil, "", fmt.Errorf("could not find appropriate release asset for your platform")
	}

	checksumPattern := getChecksumRegexPattern(asset.Name)
	var checksumAsset *GithubAsset
	for _, a := range release.Assets {
		if checksumPattern.MatchString(a.Name) {
			checksumAsset = &a
			break
		}
	}

	if checksumAsset == nil {
		return nil, nil, "", fmt.Errorf("could not find checksum file for %s", asset.Name)
	}

	version, err := extractVersion(asset.Name, assetPattern)
	if err != nil {
		return nil, nil, "", err
	}

	logger.Info("Found release: %s - %s", release.TagName, release.Name)
	logger.Info("Found asset: %s (%d KB)", asset.Name, asset.Size/1024)
	logger.Info("Version: %s", version)

	return asset, checksumAsset, version, nil
}

// downloadAssets downloads the zip and checksum files
func downloadAssets(asset, checksumAsset *GithubAsset) (string, string, error) {
	tmpDir := os.TempDir()
	zipPath := filepath.Join(tmpDir, asset.Name)
	checksumPath := filepath.Join(tmpDir, checksumAsset.Name)

	logger.Info("Downloading archive from %s...", asset.BrowserDownloadURL)
	if err := downloadFile(asset.BrowserDownloadURL, zipPath); err != nil {
		return "", "", err
	}
	logger.Info("Archive download complete")

	logger.Info("Downloading checksum file from %s...", checksumAsset.BrowserDownloadURL)
	if err := downloadFile(checksumAsset.BrowserDownloadURL, checksumPath); err != nil {
		return "", "", err
	}
	logger.Info("Checksum file download complete")

	return zipPath, checksumPath, nil
}

// verifyChecksum verifies the checksum of the downloaded archive
func verifyChecksum(zipPath, checksumPath string) error {
	logger.Info("Verifying checksum...")
	checksumContent, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}
	expectedChecksum := parseChecksumFile(string(checksumContent))
	logger.Debug("Expected checksum: %s", expectedChecksum)

	actualChecksum, err := calculateChecksum(zipPath)
	if err != nil {
		return err
	}
	logger.Debug("Actual checksum: %s", actualChecksum)

	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		os.Remove(zipPath)
		os.Remove(checksumPath)
		return fmt.Errorf("checksum verification failed! The downloaded archive may be corrupted")
	}

	logger.Info("Checksum verification passed!")
	return nil
}

// extractZip extracts a zip archive to destDir, preserving file modes.
// Protects against path traversal (zip-slip).
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("failed to resolve destination: %w", err)
	}

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("failed to resolve target: %w", err)
		}
		if !strings.HasPrefix(absTarget, absDest+string(os.PathSeparator)) && absTarget != absDest {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip entry: %w", err)
		}

		mode := f.Mode()
		if mode == 0 {
			mode = 0644
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", target, err)
		}

		// #nosec G110 - archive is checksum-verified before extraction
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return fmt.Errorf("failed to write %s: %w", target, err)
		}
		rc.Close()
		out.Close()
	}

	return nil
}

// extractBundle extracts the downloaded zip into a temp folder and returns:
// - the extracted bundle root (the folder containing the executable and _internal/)
// - the path to the versioned executable inside that bundle
func extractBundle(zipPath, version string) (string, string, error) {
	logger.Info("Extracting archive...")
	tmpDir := os.TempDir()
	extractDir := filepath.Join(tmpDir, fmt.Sprintf("rotki-core-%s-extract", version))

	if err := os.RemoveAll(extractDir); err != nil {
		return "", "", fmt.Errorf("failed to clear extract directory: %w", err)
	}
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create extract directory: %w", err)
	}

	if err := extractZip(zipPath, extractDir); err != nil {
		return "", "", err
	}

	// The archive contains a single top-level "rotki-core/" folder.
	bundleRoot := filepath.Join(extractDir, "rotki-core")
	if _, err := os.Stat(bundleRoot); os.IsNotExist(err) {
		entries, _ := os.ReadDir(extractDir)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return "", "", fmt.Errorf("could not find rotki-core folder in archive (got: %v)", names)
	}

	// The executable inside the bundle is named with version+platform suffix.
	exeName, err := findBundleExecutable(bundleRoot)
	if err != nil {
		return "", "", err
	}

	logger.Info("Extraction complete")
	return bundleRoot, filepath.Join(bundleRoot, exeName), nil
}

// findBundleExecutable returns the name of the rotki-core executable inside the extracted bundle.
func findBundleExecutable(bundleRoot string) (string, error) {
	entries, err := os.ReadDir(bundleRoot)
	if err != nil {
		return "", fmt.Errorf("failed to read bundle directory: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "rotki-core") {
			return name, nil
		}
	}
	return "", fmt.Errorf("could not find rotki-core executable in extracted bundle %s", bundleRoot)
}

// installBundle moves the extracted bundle into BinDir/rotki-core, renames the inner
// executable to a stable name, and ensures it is executable. Returns the final exe path.
func installBundle(bundleRoot, versionedExePath string) (string, error) {
	installDir := filepath.Join(BinDir, InstallDirName)
	logger.Info("Installing to %s...", installDir)

	// Remove any previous install (old onefile binary path OR existing onedir folder).
	if err := os.RemoveAll(installDir); err != nil {
		return "", fmt.Errorf("failed to remove existing install: %w", err)
	}

	if err := os.MkdirAll(BinDir, 0755); err != nil {
		return "", fmt.Errorf("failed to ensure bin directory: %w", err)
	}

	if err := os.Rename(bundleRoot, installDir); err != nil {
		// Rename across filesystems can fail; fall back to a recursive copy.
		if copyErr := copyDir(bundleRoot, installDir); copyErr != nil {
			return "", fmt.Errorf("failed to install bundle (rename: %v, copy: %w)", err, copyErr)
		}
		_ = os.RemoveAll(bundleRoot)
	}

	finalExePath := filepath.Join(installDir, executableName())
	versionedExeInInstall := filepath.Join(installDir, filepath.Base(versionedExePath))
	if versionedExeInInstall != finalExePath {
		if err := os.Rename(versionedExeInInstall, finalExePath); err != nil {
			return "", fmt.Errorf("failed to rename executable to stable name: %w", err)
		}
	}

	if err := os.Chmod(finalExePath, 0755); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	return finalExePath, nil
}

// copyDir recursively copies src into dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// cleanupTempFiles cleans up temporary files
func cleanupTempFiles(zipPath, checksumPath, version string) {
	extractDir := filepath.Join(os.TempDir(), fmt.Sprintf("rotki-core-%s-extract", version))
	if err := os.RemoveAll(extractDir); err != nil {
		logger.Warn("Failed to remove extract directory: %v", err)
	}

	if err := os.Remove(zipPath); err != nil {
		logger.Warn("Failed to remove temporary archive: %v", err)
	}

	if err := os.Remove(checksumPath); err != nil {
		logger.Warn("Failed to remove temporary checksum file: %v", err)
	}
}

// DownloadRotkiCore downloads and installs the latest rotki-core onedir bundle.
func DownloadRotkiCore() error {
	logger.Info("Starting download of rotki-core")

	release, err := prepareForDownload()
	if err != nil {
		return err
	}

	asset, checksumAsset, version, err := findReleaseAssets(release)
	if err != nil {
		return err
	}

	zipPath, checksumPath, err := downloadAssets(asset, checksumAsset)
	if err != nil {
		return err
	}

	if err := verifyChecksum(zipPath, checksumPath); err != nil {
		return err
	}

	bundleRoot, versionedExePath, err := extractBundle(zipPath, version)
	if err != nil {
		return err
	}

	finalPath, err := installBundle(bundleRoot, versionedExePath)
	if err != nil {
		return err
	}

	logger.Info("Verifying binary version...")
	if ok, err := verifyBinaryVersion(finalPath, version); err != nil {
		return fmt.Errorf("failed to verify binary version: %w", err)
	} else if !ok {
		return fmt.Errorf("binary version verification failed! Expected version %s but got a different version", version)
	}
	logger.Info("Binary version verification passed!")

	cleanupTempFiles(zipPath, checksumPath, version)

	logger.Info("rotki-core %s has been successfully installed to %s!", version, finalPath)
	return nil
}
