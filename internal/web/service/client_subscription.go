package service

import (
	"errors"
	"net/url"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/domainset"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SubscriptionProfileInput struct {
	Enabled              bool     `json:"enabled"`
	DisplayName          string   `json:"displayName"`
	Language             string   `json:"language"`
	Title                string   `json:"title"`
	LinkExpiresAt        int64    `json:"linkExpiresAt"`
	UpdateInterval       int      `json:"updateInterval"`
	AutoSelectEnabled    bool     `json:"autoSelectEnabled"`
	AutoSelectName       string   `json:"autoSelectName"`
	AutoSelectCandidates []string `json:"autoSelectCandidates"`
	ProbeURL             string   `json:"probeUrl"`
	ProbeTimeoutSeconds  int      `json:"probeTimeoutSeconds"`
	ProbeIntervalSeconds int      `json:"probeIntervalSeconds"`
	FallbackTag          string   `json:"fallbackTag"`
	SwitchThresholdMs    int      `json:"switchThresholdMs"`
	DebounceSeconds      int      `json:"debounceSeconds"`
}

type DirectDomainInput struct {
	Id      int    `json:"id"`
	Value   string `json:"value"`
	Mode    string `json:"mode"`
	Comment string `json:"comment"`
	Enabled *bool  `json:"enabled"`
}

type DirectDomainImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Invalid  []string `json:"invalid"`
}

func defaultSubscriptionProfile(clientId int, email string) *model.ClientSubscriptionProfile {
	return &model.ClientSubscriptionProfile{
		ClientId:             clientId,
		Enabled:              true,
		DisplayName:          email,
		Language:             "en",
		Title:                email,
		UpdateInterval:       60,
		AutoSelectName:       "Lowest latency",
		ProbeURL:             "https://www.gstatic.com/generate_204",
		ProbeTimeoutSeconds:  5,
		ProbeIntervalSeconds: 300,
		Status:               "pending",
	}
}

func (s *ClientService) GetSubscriptionProfileForRecord(id int) (*model.ClientSubscriptionProfile, error) {
	rec, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	var profile model.ClientSubscriptionProfile
	err = database.GetDB().Where("client_id = ?", id).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultSubscriptionProfile(id, rec.Email), nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *ClientService) GetSubscriptionProfileByEmail(email string) (*model.ClientSubscriptionProfile, error) {
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return nil, err
	}
	return s.GetSubscriptionProfileForRecord(rec.Id)
}

func (s *ClientService) SaveSubscriptionProfileForRecord(id int, input SubscriptionProfileInput) (*model.ClientSubscriptionProfile, error) {
	if _, err := s.GetByID(id); err != nil {
		return nil, err
	}
	if err := validateSubscriptionProfile(input); err != nil {
		return nil, err
	}
	if input.AutoSelectEnabled {
		providerID, err := (&SettingService{}).GetHappProviderID()
		if err != nil {
			return nil, err
		}
		if !validHappProviderID(providerID) {
			return nil, common.NewError("a valid Happ Provider ID is required for automatic lowest-delay selection")
		}
	}
	profile := model.ClientSubscriptionProfile{
		ClientId:             id,
		Enabled:              input.Enabled,
		DisplayName:          strings.TrimSpace(input.DisplayName),
		Language:             strings.TrimSpace(input.Language),
		Title:                strings.TrimSpace(input.Title),
		LinkExpiresAt:        input.LinkExpiresAt,
		UpdateInterval:       input.UpdateInterval,
		AutoSelectEnabled:    input.AutoSelectEnabled,
		AutoSelectName:       strings.TrimSpace(input.AutoSelectName),
		AutoSelectCandidates: input.AutoSelectCandidates,
		ProbeURL:             strings.TrimSpace(input.ProbeURL),
		ProbeTimeoutSeconds:  input.ProbeTimeoutSeconds,
		ProbeIntervalSeconds: input.ProbeIntervalSeconds,
		FallbackTag:          strings.TrimSpace(input.FallbackTag),
		SwitchThresholdMs:    input.SwitchThresholdMs,
		DebounceSeconds:      input.DebounceSeconds,
		Status:               "pending",
		LastError:            "",
	}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "client_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"enabled", "display_name", "language", "title", "link_expires_at", "update_interval",
				"auto_select_enabled", "auto_select_name", "auto_select_candidates", "probe_url",
				"probe_timeout_seconds", "probe_interval_seconds", "fallback_tag", "switch_threshold_ms",
				"debounce_seconds", "status", "last_error", "updated_at",
			}),
		}).Create(&profile).Error; err != nil {
			return err
		}
		return tx.Model(&model.ClientSubscriptionProfile{}).
			Where("client_id = ?", id).
			Updates(map[string]any{
				"enabled":             input.Enabled,
				"auto_select_enabled": input.AutoSelectEnabled,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetSubscriptionProfileForRecord(id)
}

func (s *ClientService) SaveSubscriptionProfileByEmail(email string, input SubscriptionProfileInput) (*model.ClientSubscriptionProfile, error) {
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return nil, err
	}
	return s.SaveSubscriptionProfileForRecord(rec.Id, input)
}

func validateSubscriptionProfile(input SubscriptionProfileInput) error {
	if len(strings.TrimSpace(input.DisplayName)) > 128 || len(strings.TrimSpace(input.Title)) > 128 {
		return common.NewError("subscription name is too long")
	}
	language := strings.TrimSpace(input.Language)
	if language == "" || len(language) > 16 {
		return common.NewError("subscription language is invalid")
	}
	if input.UpdateInterval < 1 || input.UpdateInterval > 10080 {
		return common.NewError("subscription update interval must be between 1 and 10080 minutes")
	}
	if input.ProbeTimeoutSeconds < 1 || input.ProbeTimeoutSeconds > 30 {
		return common.NewError("probe timeout must be between 1 and 30 seconds")
	}
	if input.ProbeIntervalSeconds < 30 || input.ProbeIntervalSeconds > 86400 {
		return common.NewError("probe interval must be between 30 and 86400 seconds")
	}
	probeURL, err := url.Parse(strings.TrimSpace(input.ProbeURL))
	if err != nil || probeURL.Host == "" || (probeURL.Scheme != "http" && probeURL.Scheme != "https") || probeURL.User != nil {
		return common.NewError("probe URL must be a valid HTTP or HTTPS URL")
	}
	if strings.EqualFold(strings.TrimSpace(input.FallbackTag), "direct") {
		return common.NewError("direct cannot be used as the automatic selection fallback")
	}
	if input.SwitchThresholdMs < 0 || input.SwitchThresholdMs > 60000 || input.DebounceSeconds < 0 || input.DebounceSeconds > 86400 {
		return common.NewError("automatic selection stabilization values are invalid")
	}
	for _, candidate := range input.AutoSelectCandidates {
		if !validOutboundSelector(candidate) {
			return common.NewError("automatic selection contains an invalid candidate")
		}
	}
	if input.FallbackTag != "" && !validOutboundSelector(input.FallbackTag) {
		return common.NewError("automatic selection fallback is invalid")
	}
	return nil
}

func validOutboundSelector(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

func validHappProviderID(value string) bool {
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

func (s *ClientService) ListDirectDomains(clientId int, includeGlobal bool) ([]model.DirectDomain, error) {
	query := database.GetDB().Model(&model.DirectDomain{})
	if clientId == 0 {
		query = query.Where("client_id = 0")
	} else if includeGlobal {
		query = query.Where("client_id IN ?", []int{0, clientId})
	} else {
		query = query.Where("client_id = ?", clientId)
	}
	var rows []model.DirectDomain
	if err := query.Order("client_id ASC, domain ASC, mode ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *ClientService) ListDirectDomainsByEmail(email string, includeGlobal bool) ([]model.DirectDomain, error) {
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return nil, err
	}
	return s.ListDirectDomains(rec.Id, includeGlobal)
}

func (s *ClientService) UpsertDirectDomain(clientId int, input DirectDomainInput) (*model.DirectDomain, error) {
	if clientId > 0 {
		if _, err := s.GetByID(clientId); err != nil {
			return nil, err
		}
	}
	if input.Mode != model.DirectDomainModeInclude && input.Mode != model.DirectDomainModeExclude {
		return nil, common.NewError("direct domain mode is invalid")
	}
	if clientId == 0 && input.Mode == model.DirectDomainModeExclude {
		return nil, common.NewError("global direct domain exclusions are not supported")
	}
	domain, err := domainset.Normalize(input.Value)
	if err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	row := model.DirectDomain{
		Id:            input.Id,
		ClientId:      clientId,
		Mode:          input.Mode,
		Domain:        domain.ASCII,
		DisplayDomain: domain.Display,
		Comment:       strings.TrimSpace(input.Comment),
		Enabled:       enabled,
	}
	db := database.GetDB()
	if row.Id > 0 {
		var existing model.DirectDomain
		if err := db.First(&existing, row.Id).Error; err != nil {
			return nil, err
		}
		if existing.ClientId != clientId {
			return nil, common.NewError("direct domain does not belong to this scope")
		}
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}, {Name: "mode"}, {Name: "domain"}},
		DoUpdates: clause.AssignmentColumns([]string{"display_domain", "comment", "enabled", "updated_at"}),
	}).Save(&row).Error; err != nil {
		return nil, err
	}
	if row.Id == 0 {
		if err := db.Where("client_id = ? AND mode = ? AND domain = ?", clientId, input.Mode, domain.ASCII).First(&row).Error; err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func (s *ClientService) ImportDirectDomains(clientId int, raw, mode, comment string) (DirectDomainImportResult, error) {
	result := DirectDomainImportResult{}
	if clientId > 0 {
		if _, err := s.GetByID(clientId); err != nil {
			return result, err
		}
	}
	if mode != model.DirectDomainModeInclude && mode != model.DirectDomainModeExclude {
		return result, common.NewError("direct domain mode is invalid")
	}
	if clientId == 0 && mode == model.DirectDomainModeExclude {
		return result, common.NewError("global direct domain exclusions are not supported")
	}
	domains, invalid := domainset.ParseMany(raw)
	result.Invalid = invalid
	result.Skipped = len(invalid)
	db := database.GetDB()
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, domain := range domains {
			row := model.DirectDomain{
				ClientId:      clientId,
				Mode:          mode,
				Domain:        domain.ASCII,
				DisplayDomain: domain.Display,
				Comment:       strings.TrimSpace(comment),
				Enabled:       true,
			}
			var count int64
			if err := tx.Model(&model.DirectDomain{}).
				Where("client_id = ? AND mode = ? AND domain = ?", clientId, mode, domain.ASCII).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				result.Skipped++
			} else {
				result.Imported++
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "client_id"}, {Name: "mode"}, {Name: "domain"}},
				DoUpdates: clause.AssignmentColumns([]string{"display_domain", "comment", "enabled", "updated_at"}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func (s *ClientService) ReplaceClientDirectDomains(clientId int, includes, excludes string) (DirectDomainImportResult, error) {
	result := DirectDomainImportResult{}
	if _, err := s.GetByID(clientId); err != nil {
		return result, err
	}
	includeDomains, includeInvalid := domainset.ParseMany(includes)
	excludeDomains, excludeInvalid := domainset.ParseMany(excludes)
	result.Invalid = append(includeInvalid, excludeInvalid...)
	result.Skipped = len(result.Invalid)
	rows := make([]model.DirectDomain, 0, len(includeDomains)+len(excludeDomains))
	for _, domain := range includeDomains {
		rows = append(rows, model.DirectDomain{
			ClientId:      clientId,
			Mode:          model.DirectDomainModeInclude,
			Domain:        domain.ASCII,
			DisplayDomain: domain.Display,
			Enabled:       true,
		})
	}
	for _, domain := range excludeDomains {
		rows = append(rows, model.DirectDomain{
			ClientId:      clientId,
			Mode:          model.DirectDomainModeExclude,
			Domain:        domain.ASCII,
			DisplayDomain: domain.Display,
			Enabled:       true,
		})
	}
	result.Imported = len(rows)
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client_id = ?", clientId).Delete(&model.DirectDomain{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
	return result, err
}

func (s *ClientService) DeleteDirectDomain(clientId, id int) error {
	result := database.GetDB().Where("id = ? AND client_id = ?", id, clientId).Delete(&model.DirectDomain{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
