// Package contentdb is a thin Go client for the ContentDB HTTP API
// https://content.luanti.org/help/api/.
// It only wraps the endpoints this tool actually needs

package contentdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DefaultBaseURL is the official ContentDB instance
const DefaultBaseURL = "https://content.luanti.org"

// Client talks to a ContentDB-compatible API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a Client for baseURL. An empty baseURL uses DefaultBaseURL.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: http.DefaultClient,
	}
}

// Package is one entry in a search result
// returned by GET /api/packages/.
type Package struct {
	Author           string `json:"author"`
	Name             string `json:"name"`
	Title            string `json:"title"`
	ShortDescription string `json:"short_description"`
	Type             string `json:"type"`
	Release          int    `json:"release"`
	Thumbnail        string `json:"thumbnail"`
}

// PackageDetails is the fuller shape returned by
// GET /api/packages/<author>/<name>/.
type PackageDetails struct {
	Author           string        `json:"author"`
	Name             string        `json:"name"`
	Title            string        `json:"title"`
	ShortDescription string        `json:"short_description"`
	LongDescription  string        `json:"long_description"`
	Type             string        `json:"type"`
	Release          int           `json:"release"`
	License          string        `json:"license"`
	Repo             string        `json:"repo"`
	Tags             []string      `json:"tags"`
	Provides         []string      `json:"provides"`
	Maintainers      []string      `json:"maintainers"`
	Downloads        int           `json:"downloads"`
	Score            float64       `json:"score"`
	State            string        `json:"state"`
	DevState         string        `json:"dev_state"`
	Thumbnail        string        `json:"thumbnail"`
	URL              string        `json:"url"` // download URL for the latest release
	SupportsAllGames bool          `json:"supports_all_games"`
	ContentWarnings  []string      `json:"content_warnings"`
	GameSupport      []GameSupport `json:"game_support"`
}

// GameSupport says whether a package supports or excludes one game.
type GameSupport struct {
	Confidence int     `json:"confidence"`
	Game       Package `json:"game"`
	Supports   bool    `json:"supports"`
}

// Release is one entry from
// GET /api/packages/<author>/<name>/releases/.
type Release struct {
	ID               int            `json:"id"`
	Name             string         `json:"name"`
	Title            string         `json:"title"`
	ReleaseDate      string         `json:"release_date"`
	ReleaseNotes     string         `json:"release_notes"`
	Commit           string         `json:"commit"`
	URL              string         `json:"url"` // path to the release zip, relative to BaseURL
	Size             int            `json:"size"`
	Downloads        int            `json:"downloads"`
	MinLuantiVersion *EngineVersion `json:"min_minetest_version"`
	MaxLuantiVersion *EngineVersion `json:"max_minetest_version"`
}

// EngineVersion is an engine release bound on a Release; nil means unbounded.
type EngineVersion struct {
	Name            string `json:"name"`
	ProtocolVersion int    `json:"protocol_version"`
	IsDev           bool   `json:"is_dev"`
}

// Dependency is one dependency entry from
// GET /api/packages/<author>/<name>/dependencies/
//
//	keyed by "author/name" in the response map.
type Dependency struct {
	Name       string   `json:"name"`
	IsOptional bool     `json:"is_optional"`
	Packages   []string `json:"packages"` // candidate "author/name" packages that provide Name
}

// SearchOptions filters a package search.
// Zero values are omitted from the request
type SearchOptions struct {
	Type   string // "mod", "game", "txp"
	Query  string
	Author string
	Tag    string
	Limit  int    // max results, 0 means no limit
	Sort   string // e.g. "downloads"
	Order  string // "asc" or "desc"

	Flag            []string // show packages with these content flags
	Hide            []string // hide packages with these tags or content flags
	License         []string // any of these licenses
	Game            string   // e.g. "Warr1024/nodecore"
	ProtocolVersion int
	EngineVersion   string // e.g. "5.9.0"
}

// Search runs GET /api/packages/ with the given filters
func (c *Client) Search(opts SearchOptions) ([]Package, error) {
	query := url.Values{}
	if opts.Type != "" {
		query.Set("type", opts.Type)
	}
	if opts.Query != "" {
		query.Set("q", opts.Query)
	}
	if opts.Author != "" {
		query.Set("author", opts.Author)
	}
	if opts.Tag != "" {
		query.Set("tag", opts.Tag)
	}
	if opts.Limit > 0 {
		query.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Sort != "" {
		query.Set("sort", opts.Sort)
	}
	if opts.Order != "" {
		query.Set("order", opts.Order)
	}
	for _, f := range opts.Flag {
		query.Add("flag", f)
	}
	for _, h := range opts.Hide {
		query.Add("hide", h)
	}
	for _, l := range opts.License {
		query.Add("license", l)
	}
	if opts.Game != "" {
		query.Set("game", opts.Game)
	}
	if opts.ProtocolVersion > 0 {
		query.Set("protocol_version", strconv.Itoa(opts.ProtocolVersion))
	}
	if opts.EngineVersion != "" {
		query.Set("engine_version", opts.EngineVersion)
	}

	var packages []Package
	if err := c.get("/api/packages/", query, &packages); err != nil {
		return nil, err
	}

	return packages, nil
}

// PackageDetails fetches GET /api/packages/<author>/<name>/
func (c *Client) PackageDetails(author, name string) (*PackageDetails, error) {
	path := fmt.Sprintf("/api/packages/%s/%s/", author, name)

	var details PackageDetails
	if err := c.get(path, nil, &details); err != nil {
		return nil, err
	}

	return &details, nil
}

// Releases fetches GET /api/packages/<author>/<name>/releases/
func (c *Client) Releases(author, name string) ([]Release, error) {
	path := fmt.Sprintf("/api/packages/%s/%s/releases/", author, name)

	var releases []Release
	if err := c.get(path, nil, &releases); err != nil {
		return nil, err
	}

	return releases, nil
}

// Dependencies fetches GET /api/packages/<author>/<name>/dependencies/
// The response is keyed by "author/name"
// onlyHard skips optional dependencies.
func (c *Client) Dependencies(author, name string, onlyHard bool) (map[string][]Dependency, error) {
	path := fmt.Sprintf("/api/packages/%s/%s/dependencies/", author, name)

	query := url.Values{}
	if onlyHard {
		query.Set("only_hard", "true")
	}

	var deps map[string][]Dependency
	if err := c.get(path, query, &deps); err != nil {
		return nil, err
	}

	return deps, nil
}

// UpdatesOptions filters an update check. Zero values are omitted.
type UpdatesOptions struct {
	Types           []string // sent like the client does; the server currently ignores it
	ProtocolVersion int
	EngineVersion   string // e.g. "5.9.0"
}

// Updates fetches GET /api/updates/
// a map of "author/name" to the latest release ID for that package
func (c *Client) Updates(opts UpdatesOptions) (map[string]int, error) {
	query := url.Values{}
	for _, t := range opts.Types {
		query.Add("type", t)
	}
	if opts.ProtocolVersion > 0 {
		query.Set("protocol_version", strconv.Itoa(opts.ProtocolVersion))
	}
	if opts.EngineVersion != "" {
		query.Set("engine_version", opts.EngineVersion)
	}

	var updates map[string]int
	if err := c.get("/api/updates/", query, &updates); err != nil {
		return nil, err
	}

	return updates, nil
}

// DownloadURL returns the URL for a package's latest release.
func (c *Client) DownloadURL(author, name string) string {
	return fmt.Sprintf("%s/packages/%s/%s/download/", c.BaseURL, author, name)
}

// ReleaseDownloadURL returns the URL for one specific release.
func (c *Client) ReleaseDownloadURL(author, name string, releaseID int) string {
	return fmt.Sprintf("%s/packages/%s/%s/releases/%s/download/",
		c.BaseURL, author, name, strconv.Itoa(releaseID))
}

// Download streams the file at downloadURL into dst, following redirects.
// Use DownloadURL or ReleaseDownloadURL to build downloadURL.
func (c *Client) Download(downloadURL string, dst io.Writer) error {
	resp, err := c.HTTPClient.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("contentdb: %s: unexpected status %d", downloadURL, resp.StatusCode)
	}

	_, err = io.Copy(dst, resp.Body)
	return err
}

// get issues a GET request against path (relative to c.BaseURL) with
// the given query params, and decodes the JSON response into out.
func (c *Client) get(path string, query url.Values, out interface{}) error {
	full := c.BaseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	resp, err := c.HTTPClient.Get(full)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("contentdb: %s: unexpected status %d: %s", full, resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
