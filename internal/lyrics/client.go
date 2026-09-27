// Package lyrics provides song lyrics fetching from Genius via RapidAPI.
package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Client provides lyrics fetching functionality with caching.
type Client struct {
	httpClient *http.Client
	apiKey     string
	apiHost    string
	cache      *lyricsCache
	enabled    bool
	logger     *slog.Logger
}

// LyricsResult contains the fetched lyrics and metadata.
type LyricsResult struct {
	Title      string
	Artist     string
	Lyrics     string   // Full lyrics text
	Pages      []string // Lyrics split into pages for Discord
	TotalPages int
	ArtworkURL string
	GeniusURL  string
}

// Config holds client configuration.
type Config struct {
	APIKey  string
	APIHost string
	Logger  *slog.Logger
}

// lyricsCache provides thread-safe caching.
type lyricsCache struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
	ttl   time.Duration
}

type cacheItem struct {
	result    *LyricsResult
	expiresAt time.Time
}

const (
	// PageSize is the maximum character limit for Discord embeds.
	PageSize       = 1900
	cacheTTL       = 30 * time.Minute
	requestTimeout = 10 * time.Second
	maxCacheItems  = 256
)

// NewClient creates a new lyrics client.
func NewClient(cfg Config) *Client {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		apiKey:     cfg.APIKey,
		apiHost:    cfg.APIHost,
		cache:      newCache(cacheTTL),
		enabled:    cfg.APIKey != "",
		logger:     logger,
	}
}

// IsEnabled returns whether the lyrics client is configured.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// FetchLyrics fetches lyrics for a song by title and artist.
func (c *Client) FetchLyrics(ctx context.Context, title, artist string) (*LyricsResult, error) {
	if !c.enabled {
		return nil, fmt.Errorf("lyrics API not configured")
	}

	// Parse and clean the title/artist for better search results
	cleanTitle, cleanArtist := c.parseTrackInfo(title, artist)

	c.logger.Info("fetching lyrics",
		"original_title", title,
		"original_artist", artist,
		"cleaned_title", cleanTitle,
		"cleaned_artist", cleanArtist,
	)

	// Create cache key using cleaned terms
	cacheKey := c.createCacheKey(cleanTitle, cleanArtist)

	// Check cache first
	if cached := c.cache.get(cacheKey); cached != nil {
		c.logger.Debug("lyrics found in cache", "title", cleanTitle)
		return cached, nil
	}

	// Search for the song
	songID, songInfo, err := c.searchSong(ctx, cleanTitle, cleanArtist)
	if err != nil {
		return nil, fmt.Errorf("failed to find song: %w", err)
	}

	c.logger.Info("found song on Genius",
		"song_id", songID,
		"genius_title", songInfo.title,
		"genius_artist", songInfo.artist,
		"genius_url", songInfo.geniusURL,
	)

	// Fetch lyrics by song ID
	rawLyrics, err := c.fetchLyricsHTML(ctx, songID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch lyrics: %w", err)
	}

	// Clean and parse lyrics
	cleanedLyrics := c.cleanLyrics(rawLyrics)
	pages := c.paginateLyrics(cleanedLyrics)

	result := &LyricsResult{
		Title:      songInfo.title,
		Artist:     songInfo.artist,
		Lyrics:     cleanedLyrics,
		Pages:      pages,
		TotalPages: len(pages),
		ArtworkURL: songInfo.artworkURL,
		GeniusURL:  songInfo.geniusURL,
	}

	// Cache the result
	c.cache.set(cacheKey, result)

	c.logger.Info("lyrics fetched successfully",
		"title", songInfo.title,
		"pages", len(pages),
	)

	return result, nil
}

// parseTrackInfo extracts clean song title and artist from track metadata.
// Handles cases like "Artist - Song Name (Official Video)" or "Song ft. Someone"
func (c *Client) parseTrackInfo(title, artist string) (cleanTitle, cleanArtist string) {
	cleanArtist = c.cleanArtistName(artist)

	// Check if title contains "Artist - Song" format (common in YouTube)
	if idx := strings.Index(title, " - "); idx != -1 {
		// Title format: "Artist - Song Name (extras)"
		titleArtist := strings.TrimSpace(title[:idx])
		songPart := strings.TrimSpace(title[idx+3:])

		// Clean the song part (remove video markers, etc.)
		cleanTitle = c.cleanSongTitle(songPart)

		// If the artist from title differs, prefer it (more specific)
		if titleArtist != "" && !strings.EqualFold(titleArtist, artist) {
			// Title might have "Artist ft. Someone", extract main artist
			cleanArtist = c.cleanArtistName(titleArtist)
		}
	} else {
		// No separator, just clean the title
		cleanTitle = c.cleanSongTitle(title)
	}

	// Fallback if cleaning removed everything
	if cleanTitle == "" {
		cleanTitle = title
	}
	if cleanArtist == "" {
		cleanArtist = artist
	}

	return cleanTitle, cleanArtist
}

// cleanSongTitle removes video markers and other noise from song titles.
func (c *Client) cleanSongTitle(s string) string {
	if s == "" {
		return s
	}

	// Patterns to remove from song titles (case-insensitive)
	patternsToRemove := []string{
		// Video types
		`\s*\(official\s*(music\s*)?(video|audio|visualizer|lyric\s*video)?\)`,
		`\s*\[official\s*(music\s*)?(video|audio|visualizer|lyric\s*video)?\]`,
		`\s*\(official\s*radio\s*edit\)`,
		`\s*\[official\s*radio\s*edit\]`,
		`\s*\(lyric\s*video\)`,
		`\s*\[lyric\s*video\]`,
		`\s*\(lyrics?\)`,
		`\s*\[lyrics?\]`,
		`\s*\(audio\)`,
		`\s*\[audio\]`,
		`\s*\(visualizer\)`,
		`\s*\[visualizer\]`,
		`\s*\(music\s*video\)`,
		`\s*\[music\s*video\]`,

		// Remaster/version info
		`\s*\(remaster(ed)?\s*\d*\)`,
		`\s*\[remaster(ed)?\s*\d*\]`,
		`\s*\(.*remaster.*\)`,
		`\s*\[.*remaster.*\]`,

		// Live/acoustic versions
		`\s*\(live\s*(at|from|in)?\s*[^)]*\)`,
		`\s*\[live\s*(at|from|in)?\s*[^\]]*\]`,
		`\s*\(acoustic\s*version?\)`,
		`\s*\[acoustic\s*version?\]`,

		// Extra metadata
		`\s*\(explicit\)`,
		`\s*\[explicit\]`,
		`\s*\(clean\)`,
		`\s*\[clean\]`,
		`\s*\(radio\s*edit\)`,
		`\s*\[radio\s*edit\]`,
		`\s*-\s*radio\s*edit`,
		`\s*\(single\s*version\)`,
		`\s*\[single\s*version\]`,
		`\s*\(album\s*version\)`,
		`\s*\[album\s*version\]`,
		`\s*\(extended\s*(mix|version)?\)`,
		`\s*\[extended\s*(mix|version)?\]`,
		`\s*\(original\s*mix\)`,
		`\s*\[original\s*mix\]`,

		// Years in parentheses
		`\s*\(\d{4}\)`,
		`\s*\[\d{4}\]`,

		// HD/HQ markers
		`\s*\(hd\)`,
		`\s*\[hd\]`,
		`\s*\(hq\)`,
		`\s*\[hq\]`,
		`\s*\(4k\)`,
		`\s*\[4k\]`,
	}

	result := s
	for _, pattern := range patternsToRemove {
		re := regexp.MustCompile(`(?i)` + pattern)
		result = re.ReplaceAllString(result, "")
	}

	// Clean up extra whitespace
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	return result
}

// cleanArtistName removes noise from artist names.
func (c *Client) cleanArtistName(s string) string {
	if s == "" {
		return s
	}

	// Remove featuring artists for cleaner search (keep main artist)
	patternsToRemove := []string{
		`\s+ft\.?\s+.*$`,
		`\s+feat\.?\s+.*$`,
		`\s+featuring\s+.*$`,
		`\s+x\s+.*$`,    // "Artist x Artist2"
		`\s+&\s+.*$`,    // "Artist & Artist2"
		`\s*-\s*topic$`, // YouTube auto-generated
		`\s*vevo$`,
	}

	result := s
	for _, pattern := range patternsToRemove {
		re := regexp.MustCompile(`(?i)` + pattern)
		result = re.ReplaceAllString(result, "")
	}

	result = strings.TrimSpace(result)
	return result
}

// cleanSearchTerm is kept for backward compatibility but delegates to new functions.
func (c *Client) cleanSearchTerm(s string) string {
	return c.cleanSongTitle(s)
}

// searchSong searches for a song and returns its Genius ID.
func (c *Client) searchSong(ctx context.Context, title, artist string) (int64, *songInfo, error) {
	query := fmt.Sprintf("%s %s", artist, title)
	encodedQuery := url.QueryEscape(query)

	reqURL := fmt.Sprintf("https://%s/search/?q=%s&per_page=10&page=1", c.apiHost, encodedQuery)

	c.logger.Info("searching Genius API",
		"query", query,
		"url", reqURL,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("x-rapidapi-host", c.apiHost)
	req.Header.Set("x-rapidapi-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("search API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	var searchResp searchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return 0, nil, err
	}

	// Log all search results for debugging
	c.logger.Info("Genius search results",
		"query", query,
		"num_results", len(searchResp.Hits),
	)
	for i, hit := range searchResp.Hits {
		c.logger.Info("search result",
			"index", i,
			"type", hit.Type,
			"title", hit.Result.Title,
			"artist", hit.Result.ArtistNames,
			"id", hit.Result.ID,
		)
	}

	if len(searchResp.Hits) == 0 {
		return 0, nil, fmt.Errorf("no results found for: %s", query)
	}

	// Find the best matching result
	hit, err := c.findBestMatch(searchResp.Hits, title, artist)
	if err != nil {
		return 0, nil, err
	}

	info := &songInfo{
		title:      hit.Title,
		artist:     hit.ArtistNames,
		artworkURL: hit.SongArtImageURL,
		geniusURL:  hit.URL,
	}

	return hit.ID, info, nil
}

// findBestMatch finds the best matching song from search results.
// It filters out non-songs and prefers results that match the artist.
func (c *Client) findBestMatch(hits []searchHit, title, artist string) (*searchResult, error) {
	artistLower := strings.ToLower(artist)
	titleLower := strings.ToLower(title)

	// List of artist names/patterns to exclude (usually indicate non-song content)
	excludeArtists := []string{
		"genius",
		"spotify",
		"annotated",
	}

	// List of title patterns to exclude
	excludeTitlePatterns := []string{
		"playlist",
		"behind the lyrics",
		"annotated",
		"tracklist",
	}

	var candidates []*searchResult

	// First pass: filter to only valid songs
	for _, hit := range hits {
		// Skip non-song types
		if hit.Type != "" && hit.Type != "song" {
			c.logger.Debug("skipping non-song result", "type", hit.Type, "title", hit.Result.Title)
			continue
		}

		resultArtistLower := strings.ToLower(hit.Result.ArtistNames)
		resultTitleLower := strings.ToLower(hit.Result.Title)

		// Skip results from excluded artists
		skip := false
		for _, exclude := range excludeArtists {
			if strings.Contains(resultArtistLower, exclude) {
				c.logger.Debug("skipping excluded artist", "artist", hit.Result.ArtistNames)
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Skip results with excluded title patterns
		for _, pattern := range excludeTitlePatterns {
			if strings.Contains(resultTitleLower, pattern) {
				c.logger.Debug("skipping excluded title pattern", "title", hit.Result.Title)
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		candidates = append(candidates, &hit.Result)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no song results match the requested track")
	}

	// Second pass: find the best match by artist and title similarity
	var bestMatch *searchResult
	bestScore := -1

	for _, result := range candidates {
		resultArtistLower := strings.ToLower(result.ArtistNames)
		resultTitleLower := strings.ToLower(result.Title)
		// Require evidence for the title and, when supplied, the artist.
		// An empty query must never count as a substring match.
		titleMatches := matchesSearchPhrase(resultTitleLower, titleLower)
		artistMatches := matchesSearchPhrase(resultArtistLower, artistLower)
		if !titleMatches || (artistLower != "" && !artistMatches) {
			continue
		}

		score := 0

		// Artist matches (most important)
		if artistMatches {
			score += 10
		}

		// Extract main artist (before "Ft." etc)
		mainArtist := strings.Split(resultArtistLower, "(")[0]
		mainArtist = strings.TrimSpace(mainArtist)
		if artistLower != "" && mainArtist != "" && (strings.Contains(mainArtist, artistLower) || strings.Contains(artistLower, mainArtist)) {
			score += 5
		}

		// Title matches
		if strings.Contains(resultTitleLower, titleLower) || strings.Contains(titleLower, resultTitleLower) {
			score += 5
		}

		// Exact title match (bonus)
		if resultTitleLower == titleLower {
			score += 10
		}

		c.logger.Debug("candidate score",
			"title", result.Title,
			"artist", result.ArtistNames,
			"score", score,
		)

		if score > bestScore {
			bestScore = score
			bestMatch = result
		}
	}

	if bestMatch != nil {
		c.logger.Info("selected best match",
			"title", bestMatch.Title,
			"artist", bestMatch.ArtistNames,
			"score", bestScore,
		)
		return bestMatch, nil
	}

	return nil, fmt.Errorf("no sufficiently matching song found for %q by %q", title, artist)
}

// matchesSearchPhrase requires whole words, so "Sky" does not match "Skylight".
func matchesSearchPhrase(result, query string) bool {
	normalize := func(s string) string {
		return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}), " ")
	}
	result, query = normalize(result), normalize(query)
	return result != "" && query != "" && (strings.Contains(" "+result+" ", " "+query+" ") || strings.Contains(" "+query+" ", " "+result+" "))
}

// fetchLyricsHTML fetches the raw lyrics HTML for a song ID.
func (c *Client) fetchLyricsHTML(ctx context.Context, songID int64) (string, error) {
	reqURL := fmt.Sprintf("https://%s/song/lyrics/?id=%d", c.apiHost, songID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("x-rapidapi-host", c.apiHost)
	req.Header.Set("x-rapidapi-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lyrics API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var lyricsResp lyricsResponse
	if err := json.Unmarshal(body, &lyricsResp); err != nil {
		return "", err
	}

	if lyricsResp.Lyrics.Lyrics.Body.HTML == "" {
		return "", fmt.Errorf("no lyrics found for this song")
	}

	return lyricsResp.Lyrics.Lyrics.Body.HTML, nil
}

// cleanLyrics removes HTML tags and formats the lyrics.
func (c *Client) cleanLyrics(html string) string {
	// Replace <br> tags with newlines
	br := regexp.MustCompile(`<br\s*/?>`)
	result := br.ReplaceAllString(html, "\n")

	// Remove all other HTML tags
	tags := regexp.MustCompile(`<[^>]*>`)
	result = tags.ReplaceAllString(result, "")

	// Decode HTML entities
	result = c.decodeHTMLEntities(result)

	// Clean up whitespace
	result = strings.TrimSpace(result)

	// Remove excessive blank lines (more than 2)
	multipleNewlines := regexp.MustCompile(`\n{3,}`)
	result = multipleNewlines.ReplaceAllString(result, "\n\n")

	return result
}

// decodeHTMLEntities decodes common HTML entities.
func (c *Client) decodeHTMLEntities(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&apos;", "'",
		"&nbsp;", " ",
		"&#x27;", "'",
	)
	return replacer.Replace(s)
}

// paginateLyrics splits lyrics into pages that fit Discord's character limit.
func (c *Client) paginateLyrics(lyrics string) []string {
	if utf8.RuneCountInString(lyrics) <= PageSize {
		return []string{lyrics}
	}

	var pages []string
	lines := strings.Split(lyrics, "\n")
	var currentPage []rune

	for _, line := range lines {
		runes := []rune(line)
		if len(currentPage) > 0 && len(currentPage)+len(runes)+1 > PageSize {
			pages = append(pages, strings.TrimSpace(string(currentPage)))
			currentPage = nil
		}
		for len(runes) > PageSize {
			pages = append(pages, string(runes[:PageSize]))
			runes = runes[PageSize:]
		}
		if len(currentPage) > 0 {
			currentPage = append(currentPage, '\n')
		}
		currentPage = append(currentPage, runes...)
	}

	// Don't forget the last page
	if len(currentPage) > 0 {
		pages = append(pages, strings.TrimSpace(string(currentPage)))
	}

	return pages
}

// createCacheKey creates a normalized cache key.
func (c *Client) createCacheKey(title, artist string) string {
	normalized := strings.ToLower(strings.TrimSpace(title) + "|" + strings.TrimSpace(artist))
	return normalized
}

// songInfo holds song metadata from search.
type songInfo struct {
	title      string
	artist     string
	artworkURL string
	geniusURL  string
}

// API response types.
type searchResponse struct {
	Hits []searchHit `json:"hits"`
}

type searchHit struct {
	Type   string       `json:"type"`
	Result searchResult `json:"result"`
}

type searchResult struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	ArtistNames     string `json:"artist_names"`
	SongArtImageURL string `json:"song_art_image_url"`
	URL             string `json:"url"`
}

type lyricsResponse struct {
	Lyrics struct {
		Lyrics struct {
			Body struct {
				HTML string `json:"html"`
			} `json:"body"`
		} `json:"lyrics"`
	} `json:"lyrics"`
}

// Cache methods.
func newCache(ttl time.Duration) *lyricsCache {
	return &lyricsCache{
		items: make(map[string]*cacheItem),
		ttl:   ttl,
	}
}

func (c *lyricsCache) get(key string) *LyricsResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists || time.Now().After(item.expiresAt) {
		delete(c.items, key)
		return nil
	}
	return item.result
}

func (c *lyricsCache) set(key string, result *LyricsResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	oldestKey := ""
	var oldest time.Time
	for cacheKey, item := range c.items {
		if !now.Before(item.expiresAt) {
			delete(c.items, cacheKey)
			continue
		}
		if oldestKey == "" || item.expiresAt.Before(oldest) {
			oldestKey, oldest = cacheKey, item.expiresAt
		}
	}
	if _, exists := c.items[key]; !exists && len(c.items) >= maxCacheItems {
		delete(c.items, oldestKey)
	}

	c.items[key] = &cacheItem{
		result:    result,
		expiresAt: now.Add(c.ttl),
	}
}
