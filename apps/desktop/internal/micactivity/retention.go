package micactivity

import (
	"sort"
	"strings"
	"time"

	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

const unlinkedRetention = 30 * 24 * time.Hour

func identifiedApplication(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && !strings.HasPrefix(strings.ToLower(id), "pid:")
}

// ReconcileHistory retains recording annotations independently of output-folder
// availability, and expires ended observations that never annotated saved audio.
func (m *Monitor) ReconcileHistory(history []protocol.RecordingHistoryItem, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	ranges := recordedIntervals(history)
	cutoff := now.Add(-unlinkedRetention)
	kept := make([]entry, 0, len(m.entries))
	active := map[string]int{}
	for _, item := range m.entries {
		start, err := time.Parse(time.RFC3339Nano, item.StartedAt)
		end := now
		if item.EndedAt != nil {
			end, err = time.Parse(time.RFC3339Nano, *item.EndedAt)
		}
		if !item.RecordingLinked && err == nil && overlapsRecording(ranges, start, end, item.EndedAt == nil) {
			item.RecordingLinked = true
			m.dirtyDays[item.Day] = true
		}
		if !identifiedApplication(item.AppID) || (!item.RecordingLinked && item.EndedAt != nil && err == nil && !end.After(cutoff)) {
			m.dirtyDays[item.Day] = true
			continue
		}
		if item.EndedAt == nil {
			active[item.AppID] = len(kept)
		}
		kept = append(kept, item)
	}
	m.entries, m.active = kept, active
	m.storageError = m.save()
	return m.storageError
}

type recordedInterval struct {
	start, end, latestEnd time.Time
}

func recordedIntervals(history []protocol.RecordingHistoryItem) []recordedInterval {
	ranges := []recordedInterval{}
	for _, item := range history {
		if item.AudioSource == protocol.SourceSystem {
			continue
		}
		captures := item.CapturedRanges
		if len(captures) == 0 {
			for _, chunk := range item.TranscriptChunks {
				captures = append(captures, protocol.RecordingTimeRange{StartedAt: chunk.StartedAt, EndedAt: chunk.EndedAt})
			}
		}
		if len(captures) == 0 {
			captures = []protocol.RecordingTimeRange{{StartedAt: item.StartedAt, EndedAt: item.StartedAt}}
		}
		for _, capture := range captures {
			start, startErr := recordingTime(capture.StartedAt)
			end, endErr := recordingTime(capture.EndedAt)
			if startErr == nil && endErr == nil && !end.Before(start) {
				ranges = append(ranges, recordedInterval{start: start, end: end})
			}
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start.Before(ranges[j].start) })
	var latest time.Time
	for i := range ranges {
		if ranges[i].end.After(latest) {
			latest = ranges[i].end
		}
		ranges[i].latestEnd = latest
	}
	return ranges
}

func recordingTime(value string) (time.Time, error) {
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return at, nil
	}
	// Legacy filename-derived metadata uses local wall time without an offset.
	return time.ParseInLocation("2006-01-02T15:04:05", value, time.Local)
}

func overlapsRecording(ranges []recordedInterval, start, end time.Time, active bool) bool {
	first := sort.Search(len(ranges), func(i int) bool { return !ranges[i].latestEnd.Before(start) })
	for _, interval := range ranges[first:] {
		if interval.start.After(end) || (!active && interval.start.Equal(end)) {
			break
		}
		if interval.start.Equal(interval.end) {
			if !interval.start.Before(start) && (interval.start.Before(end) || (active && interval.start.Equal(end))) {
				return true
			}
		} else if start.Before(interval.end) && interval.start.Before(end) {
			return true
		}
	}
	return false
}
