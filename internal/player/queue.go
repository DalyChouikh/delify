package player

import (
	"sync"

	"github.com/disgoorg/disgolink/v3/lavalink"
)

// Queue manages a list of tracks for playback.
type Queue struct {
	tracks []lavalink.Track
	mu     sync.Mutex
}

// NewQueue creates a new empty queue.
func NewQueue() *Queue {
	return &Queue{
		tracks: make([]lavalink.Track, 0),
	}
}

// Add appends a track to the queue.
func (q *Queue) Add(track lavalink.Track) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracks = append(q.tracks, track)
}

// Next removes and returns the first track from the queue.
func (q *Queue) Next() (lavalink.Track, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tracks) == 0 {
		return lavalink.Track{}, false
	}

	track := q.tracks[0]
	q.tracks = q.tracks[1:]
	return track, true
}

// Clear removes all tracks from the queue.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracks = make([]lavalink.Track, 0)
}

// Len returns the number of tracks in the queue.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tracks)
}

// Peek returns the first track without removing it.
func (q *Queue) Peek() (lavalink.Track, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tracks) == 0 {
		return lavalink.Track{}, false
	}

	return q.tracks[0], true
}

// Tracks returns a copy of all tracks in the queue.
func (q *Queue) Tracks() []lavalink.Track {
	q.mu.Lock()
	defer q.mu.Unlock()

	tracks := make([]lavalink.Track, len(q.tracks))
	copy(tracks, q.tracks)
	return tracks
}
