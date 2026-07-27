package service

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type IPLimitEvent struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	IP     string `json:"ip"`
}

var ipLimitEventPattern = regexp.MustCompile(`^\s*(.*?)\s+(BAN|UNBAN)\s+\[Email\]\s*=\s*(.*?)\s+\[IP\]\s*=\s*([^\s]+)`)

func (s *InboundService) GetClientIPLimitEvents(email string, limit int) ([]IPLimitEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var events []IPLimitEvent
	for _, path := range []string{xray.GetIPLimitBannedPrevLogPath(), xray.GetIPLimitBannedLogPath()} {
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			match := ipLimitEventPattern.FindStringSubmatch(scanner.Text())
			if len(match) != 5 || strings.TrimSpace(match[3]) != email {
				continue
			}
			events = append(events, IPLimitEvent{
				Time:   strings.TrimSpace(match[1]),
				Action: strings.ToLower(match[2]),
				IP:     strings.TrimSpace(match[4]),
			})
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil {
			return nil, scanErr
		}
	}
	slices.Reverse(events)
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}
