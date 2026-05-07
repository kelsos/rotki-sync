package models

// VersionInfo holds rotki version details returned by the /info endpoint.
type VersionInfo struct {
	OurVersion    string `json:"our_version"`
	LatestVersion string `json:"latest_version,omitempty"`
	DownloadURL   string `json:"download_url,omitempty"`
}

// Info is the result payload of GET /api/1/info.
type Info struct {
	Version       VersionInfo `json:"version"`
	DataDirectory string      `json:"data_directory"`
	LogLevel      string      `json:"log_level"`
}

// InfoResponse is the API envelope for the /info endpoint.
type InfoResponse = APIResponse[Info]
