package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/zionboggan/agent-time-ledger/internal/clock"
	"github.com/zionboggan/agent-time-ledger/internal/ledger"
	"github.com/zionboggan/agent-time-ledger/internal/mcp"
	"github.com/zionboggan/agent-time-ledger/internal/state"
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	store, err := state.DefaultStore()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	service := ledger.NewService(store)
	if err := run(args, stdin, stdout, stderr, service); err != nil {
		fmt.Fprintln(stderr, "atl:", err)
		return 1
	}
	return 0
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, service *ledger.Service) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "now":
		jsonOut, rest, err := extractJSONFlag(args[1:])
		if err != nil {
			return err
		}
		timezone, ok := optionalStringFlag(rest, "--tz")
		if !ok {
			timezone, _ = optionalStringFlag(rest, "--timezone")
		}
		rest = removeStringFlag(rest, "--tz", "--timezone")
		if len(rest) != 0 {
			return fmt.Errorf("unexpected argument %q", rest[0])
		}
		response, err := service.NowResponseIn(timezone)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, response)
		}
		fmt.Fprintf(stdout, "%s (%s, UTC%s)\nUTC: %s\n", response.Timestamp, response.Timezone, response.UTCOffset, response.UTCTimestamp)
		return nil
	case "session":
		return runSession(args[1:], stdout, service)
	case "mark":
		return runMark(args[1:], stdout, service)
	case "stale":
		return runStale(args[1:], stdout, service)
	case "report":
		return runReport(args[1:], stdout, service)
	case "serve-mcp":
		return mcp.Serve(stdin, stdout, stderr, service)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runSession(args []string, stdout io.Writer, service *ledger.Service) error {
	if len(args) == 0 {
		return fmt.Errorf("session command is required")
	}
	switch args[0] {
	case "start":
		name, err := stringFlag(args[1:], "--name")
		if err != nil {
			return err
		}
		status, err := service.StartSession(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "started session %s (%s)\n", status.ID, status.Name)
		return nil
	case "status":
		jsonOut, err := parseJSONFlag(args[1:])
		if err != nil {
			return err
		}
		status, err := service.SessionStatus()
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, status)
		}
		if !status.Active {
			fmt.Fprintln(stdout, "no active session")
			return nil
		}
		fmt.Fprintf(stdout, "%s (%s): %s elapsed\n", status.ID, status.Name, status.ElapsedHuman)
		return nil
	case "end":
		status, err := service.EndSession()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "ended session %s (%s): %s elapsed\n", status.ID, status.Name, status.ElapsedHuman)
		return nil
	default:
		return fmt.Errorf("unknown session command %q", args[0])
	}
}

func runMark(args []string, stdout io.Writer, service *ledger.Service) error {
	if len(args) == 0 {
		return fmt.Errorf("mark command is required")
	}
	switch args[0] {
	case "start":
		if len(args) != 2 {
			return fmt.Errorf("usage: atl mark start <name>")
		}
		mark, err := service.StartMark(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "started mark %s at %s\n", mark.Name, mark.StartedAt)
		return nil
	case "check":
		if len(args) < 2 {
			return fmt.Errorf("usage: atl mark check <name> [--json]")
		}
		jsonOut, rest, err := extractJSONFlag(args[2:])
		if err != nil {
			return err
		}
		if len(rest) != 0 {
			return fmt.Errorf("unexpected argument %q", rest[0])
		}
		mark, err := service.MarkElapsed(args[1])
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, mark)
		}
		fmt.Fprintf(stdout, "%s: %s elapsed\n", mark.Name, mark.ElapsedHuman)
		return nil
	case "list":
		marks, err := service.ListMarks()
		if err != nil {
			return err
		}
		sort.Slice(marks, func(i, j int) bool { return marks[i].Name < marks[j].Name })
		if len(marks) == 0 {
			fmt.Fprintln(stdout, "no marks")
			return nil
		}
		for _, mark := range marks {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", mark.Name, mark.StartedAt, mark.ElapsedHuman)
		}
		return nil
	case "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: atl mark delete <name>")
		}
		if err := service.DeleteMark(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "deleted mark %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown mark command %q", args[0])
	}
}

func runStale(args []string, stdout io.Writer, service *ledger.Service) error {
	jsonOut, rest, err := extractJSONFlag(args)
	if err != nil {
		return err
	}
	timestampValue, _ := optionalStringFlag(rest, "--timestamp")
	markName, _ := optionalStringFlag(rest, "--mark")
	ttlValue, err := stringFlag(rest, "--ttl")
	if err != nil {
		return err
	}
	ttl, err := clock.ParseDuration(ttlValue)
	if err != nil {
		return err
	}

	var response ledger.StaleResponse
	switch {
	case timestampValue != "" && markName != "":
		return fmt.Errorf("use either --timestamp or --mark, not both")
	case timestampValue != "":
		reference, err := clock.ParseRFC3339(timestampValue)
		if err != nil {
			return fmt.Errorf("invalid --timestamp: %w", err)
		}
		response, err = service.StaleFromTimestamp(reference, ttl)
	case markName != "":
		response, err = service.StaleFromMark(markName, ttl)
	default:
		return fmt.Errorf("either --timestamp or --mark is required")
	}
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, response)
	}
	fmt.Fprintf(stdout, "stale=%t elapsed=%s ttl=%s\n", response.Stale, response.ElapsedHuman, response.TTL)
	return nil
}

func runReport(args []string, stdout io.Writer, service *ledger.Service) error {
	jsonOut, rest, err := extractJSONFlag(args)
	if err != nil {
		return err
	}
	if len(rest) > 1 || (len(rest) == 1 && rest[0] != "today") {
		return fmt.Errorf("usage: atl report [today] [--json]")
	}
	report, err := service.ReportToday()
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, report)
	}
	fmt.Fprintf(stdout, "report %s: %d events, %d open marks, active_session=%t\n", report.Date, report.TotalEventsToday, report.OpenMarks, report.ActiveSession)
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  atl now [--json] [--tz <iana-timezone>]
  atl session start --name <name>
  atl session status [--json]
  atl session end
  atl mark start <name>
  atl mark check <name> [--json]
  atl mark list
  atl mark delete <name>
  atl stale (--timestamp <rfc3339> | --mark <name>) --ttl <duration> [--json]
  atl report [today] [--json]
  atl serve-mcp`)
}

func parseJSONFlag(args []string) (bool, error) {
	jsonOut, rest, err := extractJSONFlag(args)
	if err != nil {
		return false, err
	}
	if len(rest) != 0 {
		return false, fmt.Errorf("unexpected argument %q", rest[0])
	}
	return jsonOut, nil
}

func extractJSONFlag(args []string) (bool, []string, error) {
	jsonOut := false
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
			continue
		}
		rest = append(rest, arg)
	}
	return jsonOut, rest, nil
}

func stringFlag(args []string, name string) (string, error) {
	value, ok := optionalStringFlag(args, name)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalStringFlag(args []string, name string) (string, bool) {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1], true
		}
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimPrefix(arg, name+"="), true
		}
	}
	return "", false
}

func removeStringFlag(args []string, names ...string) []string {
	nameSet := map[string]struct{}{}
	for _, name := range names {
		nameSet[name] = struct{}{}
	}
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if _, ok := nameSet[arg]; ok {
			i++
			continue
		}
		removed := false
		for name := range nameSet {
			if strings.HasPrefix(arg, name+"=") {
				removed = true
				break
			}
		}
		if !removed {
			rest = append(rest, arg)
		}
	}
	return rest
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
