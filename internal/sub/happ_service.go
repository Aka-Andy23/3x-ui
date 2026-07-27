package sub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/clientconfig"
	"github.com/mhsanaei/3x-ui/v3/internal/util/domainset"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

type HappGeneration struct {
	JSON           string
	UserInfo       string
	ETag           string
	Title          string
	ProviderID     string
	AutoSelect     bool
	Partial        bool
	Warnings       []string
	LastModifiedAt int64
}

type happSource struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	NodeID  int    `json:"nodeId,omitempty"`
	Enabled bool   `json:"enabled"`
}

type happItem struct {
	Config       map[string]any
	Outbound     map[string]any
	Tag          string
	Remark       string
	SourceKey    string
	Sources      []happSource
	AutoEligible bool
}

func (s *SubJsonService) GetHappJSON(subId, host string, force bool) (*HappGeneration, error) {
	subReq := s.SubService.ForRequest(host)
	subReq.subscriptionBody = true
	var client model.ClientRecord
	if err := database.GetDB().Where("sub_id = ?", subId).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	profile := model.ClientSubscriptionProfile{
		ClientId:             client.Id,
		Enabled:              true,
		DisplayName:          client.Email,
		Language:             "en",
		Title:                client.Email,
		UpdateInterval:       60,
		AutoSelectName:       "Lowest latency",
		ProbeURL:             "https://www.gstatic.com/generate_204",
		ProbeTimeoutSeconds:  5,
		ProbeIntervalSeconds: 300,
		Status:               "pending",
	}
	var storedProfile model.ClientSubscriptionProfile
	if err := database.GetDB().Where("client_id = ?", client.Id).First(&storedProfile).Error; err == nil {
		profile = storedProfile
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	} else {
		if err := database.GetDB().Create(&profile).Error; err != nil {
			return nil, err
		}
	}
	now := time.Now()
	if !client.Enable || !profile.Enabled || (client.ExpiryTime > 0 && client.ExpiryTime <= now.UnixMilli()) || (profile.LinkExpiresAt > 0 && profile.LinkExpiresAt <= now.UnixMilli()) {
		return nil, nil
	}

	sources, err := subReq.getClientExternalLinksBySubId(subId)
	if err != nil {
		return nil, err
	}
	if force {
		for _, source := range sources {
			if source.Kind == model.ExternalLinkKindJSONSubscription {
				_, _ = (&serviceClientBridge{}).refresh(source.Id)
			}
		}
		sources, err = subReq.getClientExternalLinksBySubId(subId)
		if err != nil {
			return nil, err
		}
	}

	items, warnings, err := s.localHappItems(subReq, subId, host)
	if err != nil {
		s.updateHappProfileStatus(profile.Id, "error", "local subscription generation failed", "", now, force)
		return nil, err
	}
	externalItems, externalWarnings := s.externalHappItems(sources)
	items = append(items, externalItems...)
	warnings = append(warnings, externalWarnings...)
	if len(items) == 0 {
		s.updateHappProfileStatus(profile.Id, "error", "no available connections", "", now, force)
		return nil, errors.New("no available connections")
	}

	items = deduplicateHappItems(items)
	effectiveDomains, infrastructureDomains, infrastructureIPs, err := loadHappDirectRules(client.Id, host, items, subReq.nodesByID)
	if err != nil {
		return nil, err
	}
	usedRemarks := map[string]int{}
	validItems := make([]happItem, 0, len(items))
	for i := range items {
		items[i].Remark = uniqueHappRemark(items[i].Remark, usedRemarks)
		items[i].Config["remarks"] = items[i].Remark
		applyHappMetadata(items[i].Config, items[i].Sources, warnings, profile)
		applyHappDirectRules(items[i].Config, effectiveDomains, infrastructureDomains, infrastructureIPs)
		retagHappConfig(items[i].Config, items[i].Tag)
		if err := validateHappConfig(items[i].Config); err != nil {
			warnings = append(warnings, fmt.Sprintf("connection %s was excluded because Xray rejected it", items[i].Tag))
			continue
		}
		validItems = append(validItems, items[i])
	}
	items = validItems
	if len(items) == 0 {
		s.updateHappProfileStatus(profile.Id, "error", "no valid connections", "", now, force)
		return nil, errors.New("no valid connections")
	}

	documents := make([]map[string]any, 0, len(items)+1)
	for _, item := range items {
		documents = append(documents, item.Config)
	}
	if profile.AutoSelectEnabled {
		providerID, _ := (&serviceSettingBridge{}).providerID()
		if !validHappProviderIDValue(providerID) {
			s.updateHappProfileStatus(profile.Id, "error", "valid Happ Provider ID required", "", now, force)
			return nil, errors.New("valid Happ Provider ID required")
		}
		autoConfig, autoErr := buildHappAutoConfig(items, profile, effectiveDomains, infrastructureDomains, infrastructureIPs, warnings)
		if autoErr != nil {
			s.updateHappProfileStatus(profile.Id, "error", autoErr.Error(), "", now, force)
			return nil, autoErr
		}
		documents = append(documents, autoConfig)
	}

	var body []byte
	if len(documents) == 1 {
		body, err = json.MarshalIndent(documents[0], "", "  ")
	} else {
		body, err = json.MarshalIndent(documents, "", "  ")
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	traffic, _ := subReq.AggregateTrafficByEmails([]string{client.Email})
	userInfo := fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", traffic.Up, traffic.Down, traffic.Total, traffic.ExpiryTime/1000)
	providerID, _ := (&serviceSettingBridge{}).providerID()
	status := "valid"
	if len(warnings) > 0 {
		status = "partial"
	}
	s.updateHappProfileStatus(profile.Id, status, "", hash, now, force)
	lastModified := max(client.UpdatedAt, profile.UpdatedAt)
	for _, source := range sources {
		lastModified = max(lastModified, sourceUpdatedAt(source.Id))
	}
	return &HappGeneration{
		JSON:           string(body),
		UserInfo:       userInfo,
		ETag:           `"` + hash + `"`,
		Title:          firstNonEmpty(profile.Title, profile.DisplayName, client.Email),
		ProviderID:     providerID,
		AutoSelect:     profile.AutoSelectEnabled,
		Partial:        len(warnings) > 0,
		Warnings:       warnings,
		LastModifiedAt: lastModified,
	}, nil
}

func GenerateHapp(subId, host string, force bool) (*HappGeneration, error) {
	settings := &service.SettingService{}
	remarkTemplate, err := settings.GetRemarkTemplate()
	if err != nil {
		return nil, err
	}
	jsonMux, err := settings.GetSubJsonMux()
	if err != nil {
		return nil, err
	}
	jsonRules, err := settings.GetSubJsonRules()
	if err != nil {
		return nil, err
	}
	jsonFinalMask, err := settings.GetSubJsonFinalMask()
	if err != nil {
		return nil, err
	}
	subService := NewSubService(remarkTemplate)
	return NewSubJsonService(jsonMux, jsonRules, jsonFinalMask, subService).GetHappJSON(subId, host, force)
}

type serviceClientBridge struct{}

func (b *serviceClientBridge) refresh(id int) (any, error) {
	return (&service.ClientService{}).RefreshExternalJSONSource(id)
}

type serviceSettingBridge struct{}

func (b *serviceSettingBridge) providerID() (string, error) {
	return (&service.SettingService{}).GetHappProviderID()
}

func (s *SubJsonService) localHappItems(subReq *SubService, subId, host string) ([]happItem, []string, error) {
	inbounds, err := subReq.getInboundsBySubId(subId)
	if err != nil {
		return nil, nil, err
	}
	var items []happItem
	var warnings []string
	for _, inbound := range inbounds {
		clients := subReq.matchingClients(inbound, subId)
		if len(clients) == 0 {
			continue
		}
		nodeEligible := true
		nodeID := 0
		if inbound.NodeID != nil {
			nodeID = *inbound.NodeID
			node, ok := subReq.nodesByID[nodeID]
			nodeEligible = ok && node.Enable && node.Status == "online" && node.XrayState != "error"
		}
		subReq.projectThroughFallbackMaster(inbound)
		if hostEps := subReq.hostEndpoints(inbound, "json"); len(hostEps) > 0 {
			injectExternalProxy(inbound, hostEps)
		}
		for _, client := range clients {
			if !client.Enable || (client.ExpiryTime > 0 && client.ExpiryTime <= time.Now().UnixMilli()) {
				continue
			}
			configs := s.getConfig(subReq, inbound, client, host)
			for index, raw := range configs {
				var config map[string]any
				if err := json.Unmarshal(raw, &config); err != nil {
					warnings = append(warnings, fmt.Sprintf("inbound %d emitted invalid JSON", inbound.Id))
					continue
				}
				outbound := firstProxyOutbound(config)
				if outbound == nil {
					continue
				}
				key := fmt.Sprintf("local:%d:%d", inbound.Id, index)
				tag := stableHappTag(key)
				source := happSource{Type: "local", ID: strconv.Itoa(inbound.Id), Name: inbound.Remark, NodeID: nodeID, Enabled: nodeEligible}
				items = append(items, happItem{
					Config:       config,
					Outbound:     outbound,
					Tag:          tag,
					Remark:       stringValue(config["remarks"]),
					SourceKey:    key,
					Sources:      []happSource{source},
					AutoEligible: nodeEligible,
				})
			}
		}
	}
	return items, warnings, nil
}

func (s *SubJsonService) externalHappItems(sources []externalLinkEntry) ([]happItem, []string) {
	slices.SortStableFunc(sources, func(a, b externalLinkEntry) int {
		if a.Priority != b.Priority {
			return b.Priority - a.Priority
		}
		if a.SortIndex != b.SortIndex {
			return a.SortIndex - b.SortIndex
		}
		return a.Id - b.Id
	})
	var items []happItem
	var warnings []string
	for _, source := range sources {
		if !source.Enable {
			continue
		}
		sourceMeta := happSource{Type: source.Kind, ID: strconv.Itoa(source.Id), Name: source.Remark, Enabled: true}
		switch source.Kind {
		case model.ExternalLinkKindLink, model.ExternalLinkKindSubscription:
			for index, expanded := range expandEntry(source) {
				outbound := parseExternalLink(expanded.Link)
				if outbound == nil {
					warnings = append(warnings, fmt.Sprintf("external source %d contained an unsupported link", source.Id))
					continue
				}
				key := fmt.Sprintf("external:%d:%d", source.Id, index)
				remark := firstNonEmpty(expanded.Name, source.Remark, fmt.Sprintf("External %d", source.Id))
				items = append(items, happItem{
					Config:       s.newHappDocument(outbound, remark),
					Outbound:     outbound,
					Tag:          stableHappTag(key),
					Remark:       remark,
					SourceKey:    key,
					Sources:      []happSource{sourceMeta},
					AutoEligible: true,
				})
			}
		case model.ExternalLinkKindJSON, model.ExternalLinkKindJSONSubscription:
			content := source.Value
			if source.Kind == model.ExternalLinkKindJSONSubscription {
				content = source.LastGoodJSON
			}
			if strings.TrimSpace(content) == "" {
				warnings = append(warnings, fmt.Sprintf("external source %d has no last-known-good JSON", source.Id))
				continue
			}
			configs, err := clientconfig.Parse([]byte(content))
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("external source %d contains invalid JSON", source.Id))
				continue
			}
			for configIndex, config := range configs {
				for _, warning := range config.Warnings {
					warnings = append(warnings, fmt.Sprintf("external source %d: %s", source.Id, warning))
				}
				for outboundIndex, outbound := range config.Outbounds {
					key := fmt.Sprintf("json:%d:%d:%d", source.Id, configIndex, outboundIndex)
					remark := firstNonEmpty(config.Remark, source.Remark, fmt.Sprintf("External %d", source.Id))
					items = append(items, happItem{
						Config:       s.newHappDocument(outbound, remark),
						Outbound:     outbound,
						Tag:          stableHappTag(key),
						Remark:       remark,
						SourceKey:    key,
						Sources:      []happSource{sourceMeta},
						AutoEligible: true,
					})
				}
			}
		}
	}
	return items, warnings
}

func (s *SubJsonService) newHappDocument(outbound map[string]any, remark string) map[string]any {
	var config map[string]any
	_ = json.Unmarshal([]byte(defaultJson), &config)
	defaults := cloneAnySlice(config["outbounds"])
	config["outbounds"] = append([]any{cloneAnyMap(outbound)}, defaults...)
	config["remarks"] = remark
	return config
}

func deduplicateHappItems(items []happItem) []happItem {
	seen := make(map[string]int)
	out := make([]happItem, 0, len(items))
	for _, item := range items {
		fingerprint := outboundFingerprint(item.Outbound)
		if index, ok := seen[fingerprint]; ok {
			out[index].Sources = append(out[index].Sources, item.Sources...)
			out[index].AutoEligible = out[index].AutoEligible || item.AutoEligible
			continue
		}
		seen[fingerprint] = len(out)
		out = append(out, item)
	}
	return out
}

func outboundFingerprint(outbound map[string]any) string {
	copyOutbound := cloneAnyMap(outbound)
	delete(copyOutbound, "tag")
	raw, _ := json.Marshal(copyOutbound)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func stableHappTag(source string) string {
	sum := sha256.Sum256([]byte(source))
	return "proxy-" + hex.EncodeToString(sum[:8])
}

func firstProxyOutbound(config map[string]any) map[string]any {
	outbounds, _ := config["outbounds"].([]any)
	for _, raw := range outbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch strings.ToLower(stringValue(outbound["protocol"])) {
		case "vmess", "vless", "trojan", "shadowsocks", "socks", "http", "wireguard", "hysteria":
			return outbound
		}
	}
	return nil
}

func retagHappConfig(config map[string]any, tag string) {
	if outbound := firstProxyOutbound(config); outbound != nil {
		outbound["tag"] = tag
	}
	routing, _ := config["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stringValue(rule["outboundTag"]) == "proxy" {
			rule["outboundTag"] = tag
		}
	}
}

func loadHappDirectRules(clientId int, host string, items []happItem, nodes map[int]*model.Node) ([]string, []string, []string, error) {
	var rows []model.DirectDomain
	if err := database.GetDB().Where("enabled = ? AND client_id IN ?", true, []int{0, clientId}).
		Order("client_id ASC, domain ASC").Find(&rows).Error; err != nil {
		return nil, nil, nil, err
	}
	excluded := map[string]struct{}{}
	for _, row := range rows {
		if row.ClientId == clientId && row.Mode == model.DirectDomainModeExclude {
			excluded[row.Domain] = struct{}{}
		}
	}
	domainSet := map[string]struct{}{}
	for _, row := range rows {
		if row.Mode != model.DirectDomainModeInclude {
			continue
		}
		if _, skip := excluded[row.Domain]; !skip {
			domainSet[row.Domain] = struct{}{}
		}
	}
	infrastructureDomains := map[string]struct{}{}
	infrastructureIPs := map[string]struct{}{}
	addInfrastructureAddress(host, infrastructureDomains, infrastructureIPs)
	for _, node := range nodes {
		addInfrastructureAddress(node.Address, infrastructureDomains, infrastructureIPs)
	}
	for _, item := range items {
		collectOutboundAddresses(item.Outbound, infrastructureDomains, infrastructureIPs)
	}
	return sortedKeys(domainSet), sortedKeys(infrastructureDomains), sortedKeys(infrastructureIPs), nil
}

func addInfrastructureAddress(value string, domains map[string]struct{}, ips map[string]struct{}) {
	value = strings.TrimSpace(value)
	if parsed := net.ParseIP(strings.Trim(value, "[]")); parsed != nil {
		if !parsed.IsUnspecified() && !parsed.IsLoopback() {
			ips[parsed.String()] = struct{}{}
		}
		return
	}
	if normalized, err := domainset.Normalize(value); err == nil {
		domains[normalized.ASCII] = struct{}{}
	}
}

func collectOutboundAddresses(value any, domains map[string]struct{}, ips map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch strings.ToLower(key) {
			case "address", "server":
				if address, ok := child.(string); ok {
					addInfrastructureAddress(address, domains, ips)
				}
			case "endpoint":
				if endpoint, ok := child.(string); ok {
					host, _, err := net.SplitHostPort(endpoint)
					if err == nil {
						addInfrastructureAddress(host, domains, ips)
					}
				}
			}
			collectOutboundAddresses(child, domains, ips)
		}
	case []any:
		for _, child := range typed {
			collectOutboundAddresses(child, domains, ips)
		}
	}
}

func applyHappDirectRules(config map[string]any, directDomains, infrastructureDomains, infrastructureIPs []string) {
	routing, _ := config["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{"domainStrategy": "AsIs"}
		config["routing"] = routing
	}
	rules, _ := routing["rules"].([]any)
	prefix := make([]any, 0, 2)
	allDomains := append(append([]string{}, directDomains...), infrastructureDomains...)
	allDomains = uniqueStrings(allDomains)
	if len(allDomains) > 0 {
		values := make([]any, 0, len(allDomains))
		for _, domain := range allDomains {
			values = append(values, "domain:"+domain)
		}
		prefix = append(prefix, map[string]any{"type": "field", "domain": values, "outboundTag": "direct"})
	}
	if len(infrastructureIPs) > 0 {
		values := make([]any, 0, len(infrastructureIPs))
		for _, ip := range infrastructureIPs {
			values = append(values, ip)
		}
		prefix = append(prefix, map[string]any{"type": "field", "ip": values, "outboundTag": "direct"})
	}
	routing["rules"] = append(prefix, rules...)
}

func applyHappMetadata(config map[string]any, sources []happSource, warnings []string, profile model.ClientSubscriptionProfile) {
	meta, _ := config["meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["sources"] = sources
	if len(warnings) > 0 {
		meta["warnings"] = warnings
	}
	if profile.SwitchThresholdMs > 0 || profile.DebounceSeconds > 0 {
		meta["autoSelectStabilization"] = map[string]any{
			"requestedSwitchThresholdMs": profile.SwitchThresholdMs,
			"requestedDebounceSeconds":   profile.DebounceSeconds,
			"supported":                  false,
		}
	}
	config["meta"] = meta
}

func buildHappAutoConfig(items []happItem, profile model.ClientSubscriptionProfile, directDomains, infrastructureDomains, infrastructureIPs, warnings []string) (map[string]any, error) {
	selected := map[string]struct{}{}
	for _, tag := range profile.AutoSelectCandidates {
		selected[tag] = struct{}{}
	}
	candidates := make([]happItem, 0, len(items))
	for _, item := range items {
		if !item.AutoEligible {
			continue
		}
		if len(selected) > 0 {
			_, tagSelected := selected[item.Tag]
			_, sourceSelected := selected[item.SourceKey]
			prefixSelected := false
			for selector := range selected {
				if strings.HasPrefix(item.SourceKey, selector+":") {
					prefixSelected = true
					break
				}
			}
			if !tagSelected && !sourceSelected && !prefixSelected {
				continue
			}
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 {
		return nil, errors.New("no available connections")
	}
	var config map[string]any
	_ = json.Unmarshal([]byte(defaultJson), &config)
	defaults := cloneAnySlice(config["outbounds"])
	outbounds := make([]any, 0, len(candidates)+len(defaults))
	tags := make([]string, 0, len(candidates))
	sources := make([]happSource, 0, len(candidates))
	for _, candidate := range candidates {
		outbound := cloneAnyMap(candidate.Outbound)
		outbound["tag"] = candidate.Tag
		outbounds = append(outbounds, outbound)
		tags = append(tags, candidate.Tag)
		sources = append(sources, candidate.Sources...)
	}
	outbounds = append(outbounds, defaults...)
	config["outbounds"] = outbounds
	config["remarks"] = firstNonEmpty(profile.AutoSelectName, "Lowest latency")
	fallback := strings.TrimSpace(profile.FallbackTag)
	if fallback == "" {
		fallback = tags[0]
	} else if !slices.Contains(tags, fallback) {
		for _, candidate := range candidates {
			if candidate.SourceKey == fallback || strings.HasPrefix(candidate.SourceKey, fallback+":") {
				fallback = candidate.Tag
				break
			}
		}
	}
	if !slices.Contains(tags, fallback) || fallback == "direct" {
		return nil, errors.New("automatic selection fallback is not an available proxy")
	}
	balancerTag := "auto-lowest-delay"
	config["routing"] = map[string]any{
		"domainStrategy": "AsIs",
		"rules": []any{
			map[string]any{"type": "field", "network": "tcp,udp", "balancerTag": balancerTag},
		},
		"balancers": []any{
			map[string]any{
				"tag":         balancerTag,
				"selector":    tags,
				"fallbackTag": fallback,
				"strategy":    map[string]any{"type": "leastPing"},
			},
		},
	}
	sampling := 3
	if profile.DebounceSeconds > 0 {
		sampling = max(1, min(10, profile.DebounceSeconds/max(1, profile.ProbeIntervalSeconds)))
	}
	config["burstObservatory"] = map[string]any{
		"subjectSelector": tags,
		"pingConfig": map[string]any{
			"destination":  profile.ProbeURL,
			"connectivity": "",
			"interval":     strconv.Itoa(profile.ProbeIntervalSeconds) + "s",
			"sampling":     sampling,
			"timeout":      strconv.Itoa(profile.ProbeTimeoutSeconds) + "s",
			"httpMethod":   "HEAD",
		},
	}
	applyHappDirectRules(config, directDomains, infrastructureDomains, infrastructureIPs)
	applyHappMetadata(config, sources, warnings, profile)
	if err := validateHappConfig(config); err != nil {
		return nil, fmt.Errorf("Xray rejected automatic selection config: %w", err)
	}
	return config, nil
}

func validateHappConfig(config map[string]any) error {
	outbound := firstProxyOutbound(config)
	if outbound == nil {
		return errors.New("missing proxy outbound")
	}
	rawOutbound, err := json.Marshal(outbound)
	if err != nil {
		return err
	}
	if err := xray.ValidateOutboundConfig(rawOutbound); err != nil {
		return err
	}
	rawConfig, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return xray.ValidateClientConfig(rawConfig)
}

func (s *SubJsonService) updateHappProfileStatus(id int, status, message, hash string, now time.Time, force bool) {
	if id <= 0 {
		return
	}
	updates := map[string]any{
		"status":            status,
		"last_error":        message,
		"last_generated_at": now.UnixMilli(),
		"last_validated_at": now.UnixMilli(),
	}
	if hash != "" {
		updates["content_hash"] = hash
	}
	query := database.GetDB().Model(&model.ClientSubscriptionProfile{}).Where("id = ?", id)
	if !force {
		staleBefore := now.Add(-5 * time.Minute).UnixMilli()
		if hash == "" {
			query = query.Where(
				"status <> ? OR last_error <> ? OR last_generated_at < ?",
				status, message, staleBefore,
			)
		} else {
			query = query.Where(
				"status <> ? OR last_error <> ? OR content_hash <> ? OR last_generated_at < ?",
				status, message, hash, staleBefore,
			)
		}
	}
	_ = query.UpdateColumns(updates).Error
}

func sourceUpdatedAt(id int) int64 {
	var row model.ClientExternalLink
	if err := database.GetDB().Select("updated_at").First(&row, id).Error; err != nil {
		return 0
	}
	return row.UpdatedAt
}

func uniqueHappRemark(value string, used map[string]int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "Connection"
	}
	used[value]++
	if used[value] == 1 {
		return value
	}
	return fmt.Sprintf("%s (%d)", value, used[value])
}

func cloneAnyMap(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var cloned map[string]any
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func cloneAnySlice(value any) []any {
	raw, _ := json.Marshal(value)
	var cloned []any
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func validHappProviderIDValue(value string) bool {
	if len(value) != 8 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
