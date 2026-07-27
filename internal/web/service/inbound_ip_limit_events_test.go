package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetClientIPLimitEventsFiltersExactEmail(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", logDir)
	body := "2026/07/27 12:00:00   BAN   [Email] = alice [IP] = 1.1.1.1 banned.\n" +
		"2026/07/27 12:01:00   BAN   [Email] = alice-admin [IP] = 2.2.2.2 banned.\n" +
		"2026/07/27 12:02:00   UNBAN   [Email] = alice [IP] = 1.1.1.1 unbanned.\n"
	if err := os.WriteFile(filepath.Join(logDir, "3xipl-banned.log"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	events, err := (&InboundService{}).GetClientIPLimitEvents("alice", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Action != "unban" || events[1].IP != "1.1.1.1" {
		t.Fatalf("events=%+v", events)
	}
}
