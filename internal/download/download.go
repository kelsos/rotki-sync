package download

import (
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

// getAssetRegexPattern returns a regex pattern for the binary file based on the platform and architecture
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
		pattern = `rotki-core-(\d+\.\d+\.\d+)-linux$`
	case Windows:
		pattern = `rotki-core-(\d+\.\d+\.\d+)-windows\.exe$`
	default:
		logger.Fatal("Unsupported platform: %s", runtime.GOOS)
	}
	return regexp.MustCompile(pattern)
}

// getChecksumRegexPattern returns a regex pattern for the checksum file based on the binary file name
func getChecksumRegexPattern(binaryFileName string) *regexp.Regexp {
	// Escape special regex characters in the binary file name
	escapedBinaryFileName := regexp.QuoteMeta(binaryFileName)
	pattern := fmt.Sprintf("^%s\\.sha512$", escapedBinaryFileName)
	return regexp.MustCompile(pattern)
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

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write the body to file
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
	// The checksum file typically contains: "HASH *filename"
	// We need to extract just the hash part
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

	// Extract the version using a regex
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
	// Ensure bin directory exists
	if err := ensureBinDir(); err != nil {
		return nil, err
	}

	// Get latest release info
	release, err := getLatestRelease()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest release: %w", err)
	}

	return release, nil
}

// findReleaseAssets finds the appropriate asset and checksum file for the platform
func findReleaseAssets(release *GithubRelease) (*GithubAsset, *GithubAsset, string, error) {
	// Find the right asset for our platform
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

	// Find matching checksum file
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

	// Extract version from asset name
	version, err := extractVersion(asset.Name, assetPattern)
	if err != nil {
		return nil, nil, "", err
	}

	logger.Info("Found release: %s - %s", release.TagName, release.Name)
	logger.Info("Found asset: %s (%d KB)", asset.Name, asset.Size/1024)
	logger.Info("Version: %s", version)

	return asset, checksumAsset, version, nil
}

// downloadAssets downloads the binary and checksum files
func downloadAssets(asset, checksumAsset *GithubAsset) (string, string, error) {
	tmpDir := os.TempDir()
	binaryPath := filepath.Join(tmpDir, asset.Name)
	checksumPath := filepath.Join(tmpDir, checksumAsset.Name)

	logger.Info("Downloading binary from %s...", asset.BrowserDownloadURL)
	if err := downloadFile(asset.BrowserDownloadURL, binaryPath); err != nil {
		return "", "", err
	}
	logger.Info("Binary download complete")

	logger.Info("Downloading checksum file from %s...", checksumAsset.BrowserDownloadURL)
	if err := downloadFile(checksumAsset.BrowserDownloadURL, checksumPath); err != nil {
		return "", "", err
	}
	logger.Info("Checksum file download complete")

	return binaryPath, checksumPath, nil
}

// verifyChecksum verifies the checksum of the downloaded binary
func verifyChecksum(binaryPath, checksumPath string) error {
	logger.Info("Verifying checksum...")
	checksumContent, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}
	expectedChecksum := parseChecksumFile(string(checksumContent))
	logger.Debug("Expected checksum: %s", expectedChecksum)

	// Calculate actual checksum
	actualChecksum, err := calculateChecksum(binaryPath)
	if err != nil {
		return err
	}
	logger.Debug("Actual checksum: %s", actualChecksum)

	// Verify checksum
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		os.Remove(binaryPath)
		os.Remove(checksumPath)
		return fmt.Errorf("checksum verification failed! The downloaded binary may be corrupted")
	}

	logger.Info("Checksum verification passed!")
	return nil
}

// extractBinary extracts the zip file for macOS
func extractBinary(binaryPath, version string) (string, error) {
	finalBinaryPath := binaryPath
	if runtime.GOOS == Darwin {
		logger.Info("Extracting zip file...")
		tmpDir := os.TempDir()
		extractDir := filepath.Join(tmpDir, fmt.Sprintf("rotki-core-%s", version))

		if _, err := os.Stat(extractDir); err == nil {
			if err := os.RemoveAll(extractDir); err != nil {
				return "", fmt.Errorf("failed to remove existing extract directory: %w", err)
			}
		}

		if err := os.MkdirAll(extractDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create extract directory: %w", err)
		}

		// Use unzip command line utility (available on macOS)
		cmd := exec.Command("unzip", "-o", binaryPath, "-d", extractDir)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to extract zip file: %w", err)
		}

		// The binary should be inside the extracted directory
		finalBinaryPath = filepath.Join(extractDir, "rotki-core")
		if _, err := os.Stat(finalBinaryPath); os.IsNotExist(err) {
			// Try to find the binary in the extracted directory
			files, err := os.ReadDir(extractDir)
			if err != nil {
				return "", fmt.Errorf("failed to read extract directory: %w", err)
			}
			fileNames := make([]string, 0, len(files))
			for _, file := range files {
				fileNames = append(fileNames, file.Name())
			}
			return "", fmt.Errorf("could not find rotki-core binary in extracted files: %v", fileNames)
		}

		logger.Info("Extraction complete")
	}

	return finalBinaryPath, nil
}

// installBinary moves the binary to the final location and makes it executable
func installBinary(finalBinaryPath string) (string, error) {
	finalPath := filepath.Join(BinDir, "rotki-core")
	if runtime.GOOS == "windows" {
		finalPath += ".exe"
	}
	logger.Info("Installing to %s...", finalPath)

	if err := os.RemoveAll(finalPath); err != nil {
		return "", fmt.Errorf("failed to remove existing binary: %w", err)
	}

	if err := os.Rename(finalBinaryPath, finalPath); err != nil {
		// Try to copy the file instead
		src, err := os.Open(finalBinaryPath)
		if err != nil {
			return "", fmt.Errorf("failed to open source file: %w", err)
		}
		defer src.Close()

		dst, err := os.Create(finalPath)
		if err != nil {
			return "", fmt.Errorf("failed to create destination file: %w", err)
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}
	}

	// Make executable
	if err := os.Chmod(finalPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	return finalPath, nil
}

// cleanupTempFiles cleans up temporary files
func cleanupTempFiles(binaryPath, checksumPath, version string) {
	if runtime.GOOS == Darwin {
		extractDir := filepath.Join(os.TempDir(), fmt.Sprintf("rotki-core-%s", version))
		if err := os.RemoveAll(extractDir); err != nil {
			logger.Warn("Failed to remove extract directory: %v", err)
		}
	}

	if err := os.Remove(binaryPath); err != nil {
		logger.Warn("Failed to remove temporary binary file: %v", err)
	}

	if err := os.Remove(checksumPath); err != nil {
		logger.Warn("Failed to remove temporary checksum file: %v", err)
	}
}

// DownloadRotkiCore downloads and installs the latest rotki-core binary
func DownloadRotkiCore() error {
	logger.Info("Starting download of rotki-core")

	// Step 1: Prepare for download
	release, err := prepareForDownload()
	if err != nil {
		return err
	}

	// Step 2: Find release assets
	asset, checksumAsset, version, err := findReleaseAssets(release)
	if err != nil {
		return err
	}

	// Step 3: Download assets
	binaryPath, checksumPath, err := downloadAssets(asset, checksumAsset)
	if err != nil {
		return err
	}

	// Step 4: Verify checksum
	if err := verifyChecksum(binaryPath, checksumPath); err != nil {
		return err
	}

	// Step 5: Extract binary (for macOS)
	finalBinaryPath, err := extractBinary(binaryPath, version)
	if err != nil {
		return err
	}

	// Step 6: Install binary
	finalPath, err := installBinary(finalBinaryPath)
	if err != nil {
		return err
	}

	// Step 7: Verify binary version
	logger.Info("Verifying binary version...")
	if ok, err := verifyBinaryVersion(finalPath, version); err != nil {
		return fmt.Errorf("failed to verify binary version: %w", err)
	} else if !ok {
		return fmt.Errorf("binary version verification failed! Expected version %s but got a different version", version)
	}
	logger.Info("Binary version verification passed!")

	// Step 8: Clean up
	cleanupTempFiles(binaryPath, checksumPath, version)

	logger.Info("rotki-core %s has been successfully installed to %s!", version, finalPath)
	return nil
}
