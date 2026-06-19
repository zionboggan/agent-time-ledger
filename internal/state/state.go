package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StateFile  = "state.json"
	EventsFile = "events.jsonl"
	ConfigFile = "config.toml"
)

type State struct {
	ActiveSessionID string             `json:"active_session_id,omitempty"`
	Sessions        map[string]Session `json:"sessions"`
	Marks           map[string]Mark    `json:"marks"`
}

type Session struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type Mark struct {
	Name      string    `json:"name"`
	StartedAt time.Time `json:"started_at"`
}

type Event struct {
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type Store struct {
	Dir string
}

func DefaultDir() (string, error) {
	if dir := os.Getenv("ATL_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agent-time-ledger"), nil
}

func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

func DefaultStore() (*Store, error) {
	dir, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return NewStore(dir), nil
}

func NewState() State {
	return State{
		Sessions: map[string]Session{},
		Marks:    map[string]Mark{},
	}
}

func (s *Store) Ensure() error {
	if s.Dir == "" {
		return errors.New("store directory is empty")
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(s.statePath()); errors.Is(err, os.ErrNotExist) {
		if err := s.Save(NewState()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(s.eventsPath()); errors.Is(err, os.ErrNotExist) {
		file, err := os.OpenFile(s.eventsPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(s.configPath()); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(s.configPath(), []byte("# agent-time-ledger local config\n"), 0o600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

func (s *Store) Load() (State, error) {
	if err := s.Ensure(); err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(s.statePath())
	if err != nil {
		return State{}, err
	}
	if len(data) == 0 {
		return NewState(), nil
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	if st.Sessions == nil {
		st.Sessions = map[string]Session{}
	}
	if st.Marks == nil {
		st.Marks = map[string]Mark{}
	}
	return st, nil
}

func (s *Store) Save(st State) error {
	if st.Sessions == nil {
		st.Sessions = map[string]Session{}
	}
	if st.Marks == nil {
		st.Marks = map[string]Mark{}
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := s.statePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.statePath())
}

func (s *Store) AppendEvent(event Event) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	file, err := os.OpenFile(s.eventsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) ReadEvents() ([]Event, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	file, err := os.Open(s.eventsPath())
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []Event
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", EventsFile, lineNumber, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) statePath() string {
	return filepath.Join(s.Dir, StateFile)
}

func (s *Store) eventsPath() string {
	return filepath.Join(s.Dir, EventsFile)
}

func (s *Store) configPath() string {
	return filepath.Join(s.Dir, ConfigFile)
}
