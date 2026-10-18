package desktop

import (
	"errors"
	"testing"
)

type referenceRetryEngine struct {
	engine
	retries int
	err     error
}

func (e *referenceRetryEngine) RetryVoiceprintReference() error {
	e.retries++
	return e.err
}

func TestRetryVoiceprintReferenceUsesEngineWithoutResettingLearning(t *testing.T) {
	s, manager, _ := adaptationService(t)
	retry := &referenceRetryEngine{engine: s.engine}
	s.engine = retry
	before := manager.Snapshot()
	if _, err := s.RetryVoiceprintReference(); err != nil || retry.retries != 1 {
		t.Fatalf("recovery not forwarded: retries=%d err=%v", retry.retries, err)
	}
	if manager.Snapshot().Revision != before.Revision {
		t.Fatal("reference retry replaced the enrolled voiceprint")
	}
	retry.err = errors.New("capture could not reopen")
	if _, err := s.RetryVoiceprintReference(); err == nil || retry.retries != 2 {
		t.Fatal("capture errors were discarded")
	}
}
