package lyrics

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFindBestMatchRejectsUnrelatedResults(t *testing.T) {
	c := NewClient(Config{})
	for _, tc := range []struct {
		name, title, artist string
		hits                []searchHit
	}{
		{"unrelated", "Requested Song", "Requested Artist", []searchHit{{Type: "song", Result: searchResult{Title: "Other Song", ArtistNames: "Other Artist"}}}},
		{"same artist wrong song", "Requested Song", "Requested Artist", []searchHit{{Type: "song", Result: searchResult{Title: "Other Song", ArtistNames: "Requested Artist"}}}},
		{"same song wrong artist", "Requested Song", "Requested Artist", []searchHit{{Type: "song", Result: searchResult{Title: "Requested Song", ArtistNames: "Other Artist"}}}},
		{"title only unrelated", "Requested Song", "", []searchHit{{Type: "song", Result: searchResult{Title: "Other Song", ArtistNames: "Other Artist"}}}},
		{"title substring", "Sky", "Artist", []searchHit{{Type: "song", Result: searchResult{Title: "Skylight", ArtistNames: "Artist"}}}},
		{"artist substring", "Song", "A", []searchHit{{Type: "song", Result: searchResult{Title: "Song", ArtistNames: "Adele"}}}},
		{"filtered", "Playlist", "Genius", []searchHit{{Type: "song", Result: searchResult{Title: "Playlist", ArtistNames: "Genius"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if result, err := c.findBestMatch(tc.hits, tc.title, tc.artist); err == nil {
				t.Fatalf("accepted unrelated result: %+v", result)
			}
		})
	}
}

func TestFindBestMatchAcceptsMatchingSong(t *testing.T) {
	c := NewClient(Config{})
	for _, artist := range []string{"Artist", ""} {
		hits := []searchHit{{Type: "song", Result: searchResult{ID: 1, Title: "Song", ArtistNames: "Artist (Ft. Guest)"}}}
		match, err := c.findBestMatch(hits, "Song", artist)
		if err != nil || match.ID != 1 {
			t.Fatalf("matching song rejected: %+v, %v", match, err)
		}
	}
}

func TestPaginateLyricsPreservesLongUnicodeLines(t *testing.T) {
	c := NewClient(Config{})
	input := strings.Repeat("🎵", PageSize+10)
	pages := c.paginateLyrics(input)
	for _, page := range pages {
		if !utf8.ValidString(page) || utf8.RuneCountInString(page) > PageSize {
			t.Fatal("page is invalid UTF-8 or too long")
		}
	}
	if strings.Join(pages, "") != input {
		t.Fatal("long lyric line was truncated")
	}
}

func TestLyricsCacheRemovesExpiredEntries(t *testing.T) {
	c := newCache(time.Hour)
	c.set("expired", &LyricsResult{})
	c.items["expired"].expiresAt = time.Now().Add(-time.Hour)
	c.set("fresh", &LyricsResult{})
	if _, exists := c.items["expired"]; exists {
		t.Fatal("expired lyrics retained after cache insertion")
	}
}

func TestLyricsCacheStorageIsBounded(t *testing.T) {
	c := newCache(time.Hour)
	for n := 0; n < 1001; n++ {
		c.set(strings.Repeat("x", n), &LyricsResult{})
	}
	if len(c.items) > 1000 {
		t.Fatalf("lyrics cache retained %d results without a bound", len(c.items))
	}
	if c.get(strings.Repeat("x", 1000)) == nil {
		t.Fatal("newest cached lyrics were not retained")
	}
}
