package player

import (
	"sync"

	"github.com/disgoorg/disgolink/v3/lavalink"
)

// Queue manages a list of tracks for playback with requester info.
type Queue struct {
	tracks []QueuedTrack
	mu     sync.Mutex
}

// NewQueue creates a new empty queue.
func NewQueue() *Queue {
	return &Queue{
		tracks: make([]QueuedTrack, 0),
	}
}

// Add appends a track to the queue with requester info.
func (q *Queue) Add(track lavalink.Track, requestedByID, requestedBy string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracks = append(q.tracks, QueuedTrack{
		Track:         track,
		RequestedByID: requestedByID,
		RequestedBy:   requestedBy,
	})
	return len(q.tracks) // Return position (1-indexed)
}

// AddQueued appends a QueuedTrack to the queue.
func (q *Queue) AddQueued(track QueuedTrack) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracks = append(q.tracks, track)
	return len(q.tracks)
}

// Next removes and returns the first track from the queue.
func (q *Queue) Next() (QueuedTrack, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tracks) == 0 {
		return QueuedTrack{}, false
	}

	track := q.tracks[0]
	q.tracks = q.tracks[1:]
	return track, true
}

// Clear removes all tracks from the queue and returns the count.
func (q *Queue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := len(q.tracks)
	q.tracks = make([]QueuedTrack, 0)
	return count
}

// Len returns the number of tracks in the queue.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tracks)
}

// Peek returns the first track without removing it.
func (q *Queue) Peek() (QueuedTrack, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tracks) == 0 {
		return QueuedTrack{}, false
	}

	return q.tracks[0], true
}

// Tracks returns a copy of all tracks in the queue.
func (q *Queue) Tracks() []QueuedTrack {
	q.mu.Lock()
	defer q.mu.Unlock()

	tracks := make([]QueuedTrack, len(q.tracks))
	copy(tracks, q.tracks)
	return tracks
}

// GetPage returns a page of tracks for display (0-indexed page).
func (q *Queue) GetPage(page, pageSize int) ([]QueuedTrack, int, int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	total := len(q.tracks)
	if total == 0 {
		return nil, 0, 0
	}

	totalPages := (total + pageSize - 1) / pageSize
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	result := make([]QueuedTrack, end-start)
	copy(result, q.tracks[start:end])

	return result, page + 1, totalPages // Return 1-indexed page
}

// TotalDuration calculates the total duration of all tracks in the queue.
func (q *Queue) TotalDuration() lavalink.Duration {
	q.mu.Lock()
	defer q.mu.Unlock()

	var total lavalink.Duration
	for _, t := range q.tracks {
		total += t.Track.Info.Length
	}
	return total
}
