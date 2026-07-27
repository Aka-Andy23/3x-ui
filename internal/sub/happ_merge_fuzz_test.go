package sub

import (
	"fmt"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/util/clientconfig"
)

func FuzzHappMerge(f *testing.F) {
	f.Add([]byte(`{"remarks":"seed","outbounds":[{"tag":"a","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}},{"tag":"b","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}]}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		configs, err := clientconfig.Parse(data)
		if err != nil {
			return
		}
		items := make([]happItem, 0)
		for configIndex, config := range configs {
			for outboundIndex, outbound := range config.Outbounds {
				sourceKey := fmt.Sprintf("json:fuzz:%d:%d", configIndex, outboundIndex)
				items = append(items, happItem{
					Outbound:     outbound,
					Tag:          stableHappTag(sourceKey),
					SourceKey:    sourceKey,
					AutoEligible: true,
				})
			}
		}
		merged := deduplicateHappItems(items)
		seen := make(map[string]struct{}, len(merged))
		for _, item := range merged {
			fingerprint := outboundFingerprint(item.Outbound)
			if _, exists := seen[fingerprint]; exists {
				t.Fatalf("duplicate fingerprint survived: %s", fingerprint)
			}
			seen[fingerprint] = struct{}{}
			if item.Tag != stableHappTag(item.SourceKey) {
				t.Fatalf("unstable tag %q for %q", item.Tag, item.SourceKey)
			}
		}
	})
}
