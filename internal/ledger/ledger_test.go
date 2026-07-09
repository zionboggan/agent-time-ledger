package ledger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zionboggan/agent-time-ledger/internal/state"
)

func TestNowResponseIsRFC3339(t *testing.T) {
	service := testService(t, time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC))
	response, err := service.NowResponse()
	if err != nil {
		t.Fatalf("NowResponse returned error: %v", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, response.Timestamp); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", response.Timestamp, err)
	}
	if _, err := time.Parse(time.RFC3339Nano, response.UTCTimestamp); err != nil {
		t.Fatalf("utc timestamp %q is not RFC3339: %v", response.UTCTimestamp, err)
	}
	if response.Confidence != "host_clock" {
		t.Fatalf("confidence = %q, want host_clock", response.Confidence)
	}
}

func TestNowResponseTimezone(t *testing.T) {
	service := testService(t, time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC))
	response, err := service.NowResponseIn("America/Chicago")
	if err != nil {
		t.Fatalf("NowResponseIn returned error: %v", err)
	}
	if response.Timezone != "America/Chicago" {
		t.Fatalf("timezone = %q, want America/Chicago", response.Timezone)
	}
	if response.UTCOffset != "-05:00" {
		t.Fatalf("utc offset = %q, want -05:00", response.UTCOffset)
	}
	if response.Timestamp != "2026-06-19T01:30:00-05:00" {
		t.Fatalf("timestamp = %q, want Central daylight timestamp", response.Timestamp)
	}
	if response.UTCTimestamp != "2026-06-19T06:30:00Z" {
		t.Fatalf("utc timestamp = %q, want 2026-06-19T06:30:00Z", response.UTCTimestamp)
	}
}

func TestStaleCheckFreshBeforeTTL(t *testing.T) {
	now := time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC)
	service := testService(t, now)
	response, err := service.StaleFromTimestamp(now.Add(-14*time.Minute), 15*time.Minute)
	if err != nil {
		t.Fatalf("StaleFromTimestamp returned error: %v", err)
	}
	if response.Stale {
		t.Fatal("expected fresh before TTL")
	}
}

func TestStaleCheckStaleAfterTTL(t *testing.T) {
	now := time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC)
	service := testService(t, now)
	response, err := service.StaleFromTimestamp(now.Add(-16*time.Minute), 15*time.Minute)
	if err != nil {
		t.Fatalf("StaleFromTimestamp returned error: %v", err)
	}
	if !response.Stale {
		t.Fatal("expected stale after TTL")
	}
}

func TestMissingMarkError(t *testing.T) {
	service := testService(t, time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC))
	if _, err := service.MarkElapsed("missing"); err == nil {
		t.Fatal("expected missing mark error")
	}
}

func TestDeleteMissingMarkError(t *testing.T) {
	service := testService(t, time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC))
	if err := service.DeleteMark("missing"); err == nil {
		t.Fatal("expected missing mark error")
	}
}

func TestSessionStatusNoActiveSession(t *testing.T) {
	service := testService(t, time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC))
	status, err := service.SessionStatus()
	if err != nil {
		t.Fatalf("SessionStatus returned error: %v", err)
	}
	if status.Active {
		t.Fatal("expected no active session")
	}
}

func TestStatePersistence(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC)
	first := NewService(state.NewStore(dir))
	first.Now = func() time.Time { return start }
	if _, err := first.StartSession("build"); err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	if _, err := first.StartMark("compile"); err != nil {
		t.Fatalf("StartMark returned error: %v", err)
	}

	second := NewService(state.NewStore(dir))
	second.Now = func() time.Time { return start.Add(2 * time.Hour) }
	status, err := second.SessionStatus()
	if err != nil {
		t.Fatalf("SessionStatus returned error: %v", err)
	}
	if !status.Active || status.Name != "build" {
		t.Fatalf("unexpected persisted status: %+v", status)
	}
	if status.ElapsedSeconds != 7200 {
		t.Fatalf("elapsed = %f, want 7200", status.ElapsedSeconds)
	}
	mark, err := second.MarkElapsed("compile")
	if err != nil {
		t.Fatalf("MarkElapsed returned error: %v", err)
	}
	if mark.ElapsedSeconds != 7200 {
		t.Fatalf("mark elapsed = %f, want 7200", mark.ElapsedSeconds)
	}
}

func TestListMarksEmpty(t *testing.T) {
	service := testService(t, time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC))
	marks, err := service.ListMarks()
	if err != nil {
		t.Fatalf("ListMarks returned error: %v", err)
	}
	if len(marks) != 0 {
		t.Fatalf("expected 0 marks, got %d", len(marks))
	}
}

func TestListMarksReturnsAll(t *testing.T) {
	now := time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC)
	service := testService(t, now)
	if _, err := service.StartMark("build"); err != nil {
		t.Fatalf("StartMark(build) returned error: %v", err)
	}
	if _, err := service.StartMark("test"); err != nil {
		t.Fatalf("StartMark(test) returned error: %v", err)
	}
	marks, err := service.ListMarks()
	if err != nil {
		t.Fatalf("ListMarks returned error: %v", err)
	}
	if len(marks) != 2 {
		t.Fatalf("expected 2 marks, got %d", len(marks))
	}
	names := map[string]struct{}{}
	for _, m := range marks {
		names[m.Name] = struct{}{}
	}
	if _, ok := names["build"]; !ok {
		t.Fatal("mark 'build' not in ListMarks result")
	}
	if _, ok := names["test"]; !ok {
		t.Fatal("mark 'test' not in ListMarks result")
	}
}

func TestListMarksExcludesDeleted(t *testing.T) {
	now := time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC)
	service := testService(t, now)
	if _, err := service.StartMark("build"); err != nil {
		t.Fatalf("StartMark returned error: %v", err)
	}
	if _, err := service.StartMark("deploy"); err != nil {
		t.Fatalf("StartMark returned error: %v", err)
	}
	if err := service.DeleteMark("deploy"); err != nil {
		t.Fatalf("DeleteMark returned error: %v", err)
	}
	marks, err := service.ListMarks()
	if err != nil {
		t.Fatalf("ListMarks returned error: %v", err)
	}
	if len(marks) != 1 || marks[0].Name != "build" {
		t.Fatalf("expected [build], got %v", marks)
	}
}

func TestJSONLEventValidity(t *testing.T) {
	dir := t.TempDir()
	service := NewService(state.NewStore(dir))
	service.Now = func() time.Time {
		return time.Date(2026, 6, 19, 6, 30, 0, 0, time.UTC)
	}
	if _, err := service.StartSession("build"); err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	if _, err := service.StartMark("compile"); err != nil {
		t.Fatalf("StartMark returned error: %v", err)
	}
	if err := service.DeleteMark("compile"); err != nil {
		t.Fatalf("DeleteMark returned error: %v", err)
	}
	if _, err := service.StaleFromTimestamp(service.Now().Add(-2*time.Hour), time.Hour); err != nil {
		t.Fatalf("StaleFromTimestamp returned error: %v", err)
	}

	file, err := os.Open(filepath.Join(dir, state.EventsFile))
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
		var event state.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("event line %d is invalid JSON: %v", count, err)
		}
		if event.Type == "" || event.Timestamp.IsZero() {
			t.Fatalf("event line %d missing type or timestamp: %+v", count, event)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan events: %v", err)
	}
	if count != 4 {
		t.Fatalf("event count = %d, want 4", count)
	}
}

func testService(t *testing.T, now time.Time) *Service {
	t.Helper()
	service := NewService(state.NewStore(t.TempDir()))
	service.Now = func() time.Time { return now }
	return service
}
