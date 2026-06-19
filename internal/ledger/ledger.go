package ledger

import (
	"fmt"
	"time"

	"github.com/zionboggan/agent-time-ledger/internal/clock"
	"github.com/zionboggan/agent-time-ledger/internal/state"
)

type Service struct {
	Store *state.Store
	Now   func() time.Time
}

type NowResponse struct {
	Timestamp  string `json:"timestamp"`
	Unix       int64  `json:"unix"`
	Timezone   string `json:"timezone"`
	Confidence string `json:"confidence"`
}

type SessionStatusResponse struct {
	Active         bool    `json:"active"`
	ID             string  `json:"id,omitempty"`
	Name           string  `json:"name,omitempty"`
	StartedAt      string  `json:"started_at,omitempty"`
	EndedAt        string  `json:"ended_at,omitempty"`
	ElapsedSeconds float64 `json:"elapsed_seconds,omitempty"`
	ElapsedHuman   string  `json:"elapsed_human,omitempty"`
	Confidence     string  `json:"confidence"`
}

type MarkElapsedResponse struct {
	Name           string  `json:"name"`
	StartedAt      string  `json:"started_at"`
	Now            string  `json:"now"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	ElapsedHuman   string  `json:"elapsed_human"`
	Confidence     string  `json:"confidence"`
}

type MarkSummary struct {
	Name           string  `json:"name"`
	StartedAt      string  `json:"started_at"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	ElapsedHuman   string  `json:"elapsed_human"`
	Confidence     string  `json:"confidence"`
}

type StaleResponse struct {
	CheckedAt      string  `json:"checked_at"`
	ReferenceTime  string  `json:"reference_time"`
	TTL            string  `json:"ttl"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	ElapsedHuman   string  `json:"elapsed_human"`
	Stale          bool    `json:"stale"`
	Confidence     string  `json:"confidence"`
}

type ReportResponse struct {
	Date             string         `json:"date"`
	GeneratedAt      string         `json:"generated_at"`
	ActiveSession    bool           `json:"active_session"`
	ActiveSessionID  string         `json:"active_session_id,omitempty"`
	SessionsStarted  int            `json:"sessions_started"`
	SessionsEnded    int            `json:"sessions_ended"`
	OpenMarks        int            `json:"open_marks"`
	EventsByType     map[string]int `json:"events_by_type"`
	TotalEventsToday int            `json:"total_events_today"`
	Confidence       string         `json:"confidence"`
}

func NewService(store *state.Store) *Service {
	return &Service{Store: store, Now: clock.Now}
}

func (s *Service) NowResponse() NowResponse {
	now := s.now()
	return NowResponse{
		Timestamp:  clock.FormatRFC3339(now),
		Unix:       now.Unix(),
		Timezone:   "UTC",
		Confidence: clock.ConfidenceHostClock,
	}
}

func (s *Service) StartSession(name string) (SessionStatusResponse, error) {
	if name == "" {
		return SessionStatusResponse{}, fmt.Errorf("session name is required")
	}
	st, err := s.Store.Load()
	if err != nil {
		return SessionStatusResponse{}, err
	}
	if st.ActiveSessionID != "" {
		return SessionStatusResponse{}, fmt.Errorf("session already active: %s", st.ActiveSessionID)
	}
	now := s.now()
	id := "sess_" + now.Format("20060102T150405.000000000Z")
	session := state.Session{ID: id, Name: name, StartedAt: now}
	st.ActiveSessionID = id
	st.Sessions[id] = session
	if err := s.Store.Save(st); err != nil {
		return SessionStatusResponse{}, err
	}
	if err := s.Store.AppendEvent(state.Event{
		Timestamp: now,
		Type:      "session_start",
		Fields:    map[string]any{"session_id": id, "name": name},
	}); err != nil {
		return SessionStatusResponse{}, err
	}
	return s.sessionResponse(session, now), nil
}

func (s *Service) SessionStatus() (SessionStatusResponse, error) {
	st, err := s.Store.Load()
	if err != nil {
		return SessionStatusResponse{}, err
	}
	if st.ActiveSessionID == "" {
		return SessionStatusResponse{Active: false, Confidence: clock.ConfidenceWallFallback}, nil
	}
	session, ok := st.Sessions[st.ActiveSessionID]
	if !ok {
		return SessionStatusResponse{}, fmt.Errorf("active session %q not found in state", st.ActiveSessionID)
	}
	return s.sessionResponse(session, s.now()), nil
}

func (s *Service) EndSession() (SessionStatusResponse, error) {
	st, err := s.Store.Load()
	if err != nil {
		return SessionStatusResponse{}, err
	}
	if st.ActiveSessionID == "" {
		return SessionStatusResponse{}, fmt.Errorf("no active session")
	}
	session, ok := st.Sessions[st.ActiveSessionID]
	if !ok {
		return SessionStatusResponse{}, fmt.Errorf("active session %q not found in state", st.ActiveSessionID)
	}
	now := s.now()
	session.EndedAt = &now
	st.Sessions[session.ID] = session
	st.ActiveSessionID = ""
	if err := s.Store.Save(st); err != nil {
		return SessionStatusResponse{}, err
	}
	if err := s.Store.AppendEvent(state.Event{
		Timestamp: now,
		Type:      "session_end",
		Fields:    map[string]any{"session_id": session.ID, "name": session.Name},
	}); err != nil {
		return SessionStatusResponse{}, err
	}
	return s.sessionResponse(session, now), nil
}

func (s *Service) StartMark(name string) (MarkElapsedResponse, error) {
	if name == "" {
		return MarkElapsedResponse{}, fmt.Errorf("mark name is required")
	}
	st, err := s.Store.Load()
	if err != nil {
		return MarkElapsedResponse{}, err
	}
	now := s.now()
	mark := state.Mark{Name: name, StartedAt: now}
	st.Marks[name] = mark
	if err := s.Store.Save(st); err != nil {
		return MarkElapsedResponse{}, err
	}
	if err := s.Store.AppendEvent(state.Event{
		Timestamp: now,
		Type:      "mark_start",
		Fields:    map[string]any{"name": name},
	}); err != nil {
		return MarkElapsedResponse{}, err
	}
	return s.markResponse(mark, now), nil
}

func (s *Service) MarkElapsed(name string) (MarkElapsedResponse, error) {
	st, err := s.Store.Load()
	if err != nil {
		return MarkElapsedResponse{}, err
	}
	mark, ok := st.Marks[name]
	if !ok {
		return MarkElapsedResponse{}, fmt.Errorf("mark %q not found", name)
	}
	return s.markResponse(mark, s.now()), nil
}

func (s *Service) ListMarks() ([]MarkSummary, error) {
	st, err := s.Store.Load()
	if err != nil {
		return nil, err
	}
	now := s.now()
	marks := make([]MarkSummary, 0, len(st.Marks))
	for _, mark := range st.Marks {
		elapsed := now.Sub(mark.StartedAt)
		marks = append(marks, MarkSummary{
			Name:           mark.Name,
			StartedAt:      clock.FormatRFC3339(mark.StartedAt),
			ElapsedSeconds: elapsed.Seconds(),
			ElapsedHuman:   clock.FormatDuration(elapsed),
			Confidence:     clock.ConfidenceWallFallback,
		})
	}
	return marks, nil
}

func (s *Service) DeleteMark(name string) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	if _, ok := st.Marks[name]; !ok {
		return fmt.Errorf("mark %q not found", name)
	}
	delete(st.Marks, name)
	now := s.now()
	if err := s.Store.Save(st); err != nil {
		return err
	}
	return s.Store.AppendEvent(state.Event{
		Timestamp: now,
		Type:      "mark_delete",
		Fields:    map[string]any{"name": name},
	})
}

func (s *Service) StaleFromTimestamp(reference time.Time, ttl time.Duration) (StaleResponse, error) {
	return s.stale(reference.UTC(), ttl, map[string]any{"source": "timestamp"})
}

func (s *Service) StaleFromMark(name string, ttl time.Duration) (StaleResponse, error) {
	st, err := s.Store.Load()
	if err != nil {
		return StaleResponse{}, err
	}
	mark, ok := st.Marks[name]
	if !ok {
		return StaleResponse{}, fmt.Errorf("mark %q not found", name)
	}
	return s.stale(mark.StartedAt, ttl, map[string]any{"source": "mark", "name": name})
}

func (s *Service) LedgerEvent(note string) error {
	now := s.now()
	return s.Store.AppendEvent(state.Event{
		Timestamp: now,
		Type:      "manual_note",
		Fields:    map[string]any{"note": note},
	})
}

func (s *Service) ReportToday() (ReportResponse, error) {
	st, err := s.Store.Load()
	if err != nil {
		return ReportResponse{}, err
	}
	events, err := s.Store.ReadEvents()
	if err != nil {
		return ReportResponse{}, err
	}

	now := s.now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	report := ReportResponse{
		Date:            start.Format("2006-01-02"),
		GeneratedAt:     clock.FormatRFC3339(now),
		ActiveSession:   st.ActiveSessionID != "",
		ActiveSessionID: st.ActiveSessionID,
		OpenMarks:       len(st.Marks),
		EventsByType:    map[string]int{},
		Confidence:      clock.ConfidenceWallFallback,
	}
	for _, event := range events {
		if event.Timestamp.Before(start) || !event.Timestamp.Before(end) {
			continue
		}
		report.EventsByType[event.Type]++
		report.TotalEventsToday++
		switch event.Type {
		case "session_start":
			report.SessionsStarted++
		case "session_end":
			report.SessionsEnded++
		}
	}
	return report, nil
}

func (s *Service) stale(reference time.Time, ttl time.Duration, fields map[string]any) (StaleResponse, error) {
	if ttl <= 0 {
		return StaleResponse{}, fmt.Errorf("ttl must be greater than zero")
	}
	now := s.now()
	elapsed := now.Sub(reference)
	response := StaleResponse{
		CheckedAt:      clock.FormatRFC3339(now),
		ReferenceTime:  clock.FormatRFC3339(reference),
		TTL:            ttl.String(),
		ElapsedSeconds: elapsed.Seconds(),
		ElapsedHuman:   clock.FormatDuration(elapsed),
		Stale:          elapsed > ttl,
		Confidence:     clock.ConfidenceWallFallback,
	}
	eventFields := map[string]any{
		"reference_time":  response.ReferenceTime,
		"ttl":             response.TTL,
		"elapsed_seconds": response.ElapsedSeconds,
		"stale":           response.Stale,
	}
	for key, value := range fields {
		eventFields[key] = value
	}
	if err := s.Store.AppendEvent(state.Event{
		Timestamp: now,
		Type:      "stale_check",
		Fields:    eventFields,
	}); err != nil {
		return StaleResponse{}, err
	}
	return response, nil
}

func (s *Service) sessionResponse(session state.Session, now time.Time) SessionStatusResponse {
	end := now
	endedAt := ""
	active := session.EndedAt == nil
	if session.EndedAt != nil {
		end = *session.EndedAt
		endedAt = clock.FormatRFC3339(*session.EndedAt)
	}
	elapsed := end.Sub(session.StartedAt)
	return SessionStatusResponse{
		Active:         active,
		ID:             session.ID,
		Name:           session.Name,
		StartedAt:      clock.FormatRFC3339(session.StartedAt),
		EndedAt:        endedAt,
		ElapsedSeconds: elapsed.Seconds(),
		ElapsedHuman:   clock.FormatDuration(elapsed),
		Confidence:     clock.ConfidenceWallFallback,
	}
}

func (s *Service) markResponse(mark state.Mark, now time.Time) MarkElapsedResponse {
	elapsed := now.Sub(mark.StartedAt)
	return MarkElapsedResponse{
		Name:           mark.Name,
		StartedAt:      clock.FormatRFC3339(mark.StartedAt),
		Now:            clock.FormatRFC3339(now),
		ElapsedSeconds: elapsed.Seconds(),
		ElapsedHuman:   clock.FormatDuration(elapsed),
		Confidence:     clock.ConfidenceWallFallback,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return clock.Now()
}
