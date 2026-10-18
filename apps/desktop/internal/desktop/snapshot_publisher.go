package desktop

import (
	"reflect"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

type snapshotPublication uint8

const (
	publishNothing snapshotPublication = iota
	publishFullSnapshot
	publishStatusSnapshot
)

// Only publishLoop owns this state. API bootstrap responses remain complete,
// including for a newly opened or reloaded window between history changes.
type snapshotPublisher struct {
	previous       *protocol.DesktopSnapshot
	historyVersion uint64
}

func (p *snapshotPublisher) next(snapshot protocol.DesktopSnapshot, historyVersion uint64) snapshotPublication {
	publication := publishFullSnapshot
	if p.previous != nil {
		left, right := snapshotData(*p.previous), snapshotData(snapshot)
		if historyVersion != 0 && historyVersion == p.historyVersion {
			left.History, right.History = nil, nil
		}
		if reflect.DeepEqual(left, right) {
			publication = publishNothing
			if !reflect.DeepEqual(p.previous.Status, snapshot.Status) || !reflect.DeepEqual(p.previous.Voiceprint, snapshot.Voiceprint) {
				publication = publishStatusSnapshot
			}
		}
	}
	p.previous = &snapshot
	p.historyVersion = historyVersion
	return publication
}

func snapshotData(snapshot protocol.DesktopSnapshot) protocol.DesktopSnapshot {
	snapshot.Status = protocol.RecorderStatusSnapshot{}
	snapshot.Voiceprint = protocol.VoiceprintSnapshot{}
	return snapshot
}
