package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/clientconfig"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/link"
	"github.com/mhsanaei/3x-ui/v3/internal/util/securefetch"

	"gorm.io/gorm"
)

// ExternalLinkInput is one row from the client form's Links tab.
type ExternalLinkInput struct {
	Id                    int    `json:"id"`
	Kind                  string `json:"kind"`
	Value                 string `json:"value"`
	Remark                string `json:"remark"`
	Comment               string `json:"comment"`
	Enabled               *bool  `json:"enabled"`
	Priority              int    `json:"priority"`
	UpdateIntervalMinutes int    `json:"updateIntervalMinutes"`
	TimeoutSeconds        int    `json:"timeoutSeconds"`
	MaxResponseBytes      int64  `json:"maxResponseBytes"`
	MaxRedirects          int    `json:"maxRedirects"`
}

func (s *ClientService) GetExternalLinksForRecord(id int) ([]model.ClientExternalLink, error) {
	var rows []model.ClientExternalLink
	if err := database.GetDB().
		Where("client_id = ?", id).
		Order("sort_index ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// normalizeExternalLinks validates and orders the incoming rows. A "link" must
// parse to a supported share-link scheme; a "subscription" must be an http(s)
// URL. Blank values are dropped; an invalid value is a hard error so the
// operator gets immediate feedback instead of a silently missing config.
func normalizeExternalLinks(inputs []ExternalLinkInput) ([]model.ClientExternalLink, error) {
	out := make([]model.ClientExternalLink, 0, len(inputs))
	for _, in := range inputs {
		value := strings.TrimSpace(in.Value)
		if value == "" {
			continue
		}
		kind := strings.TrimSpace(in.Kind)
		switch kind {
		case model.ExternalLinkKindSubscription:
			if !isHTTPURL(value) {
				return nil, common.NewError("external subscription must be an http(s) URL")
			}
		case model.ExternalLinkKindLink, "":
			kind = model.ExternalLinkKindLink
			if _, err := link.ParseLink(value); err != nil {
				return nil, common.NewError("unsupported or invalid share link")
			}
		case model.ExternalLinkKindJSON:
			if _, err := clientconfig.Parse([]byte(value)); err != nil {
				return nil, err
			}
			var formatted bytes.Buffer
			if err := json.Indent(&formatted, []byte(value), "", "  "); err != nil {
				return nil, err
			}
			value = formatted.String()
		case model.ExternalLinkKindJSONSubscription:
			if _, err := securefetch.ValidateURL(value); err != nil {
				return nil, err
			}
		default:
			return nil, common.NewError("unknown external link kind: " + kind)
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		updateInterval := in.UpdateIntervalMinutes
		if updateInterval <= 0 {
			updateInterval = 60
		}
		if updateInterval > 10080 {
			return nil, common.NewError("external source update interval is too large")
		}
		timeoutSeconds := in.TimeoutSeconds
		if timeoutSeconds <= 0 {
			timeoutSeconds = 8
		}
		if timeoutSeconds > 60 {
			return nil, common.NewError("external source timeout is too large")
		}
		maxBytes := in.MaxResponseBytes
		if maxBytes <= 0 {
			maxBytes = 2 << 20
		}
		if maxBytes > clientconfig.MaxDocumentBytes {
			return nil, common.NewError("external source response limit is too large")
		}
		maxRedirects := in.MaxRedirects
		if maxRedirects < 0 || maxRedirects > 5 {
			return nil, common.NewError("external source redirect limit must be between 0 and 5")
		}
		out = append(out, model.ClientExternalLink{
			Id:                    in.Id,
			Kind:                  kind,
			Value:                 value,
			Remark:                strings.TrimSpace(in.Remark),
			Comment:               strings.TrimSpace(in.Comment),
			Enabled:               enabled,
			Priority:              in.Priority,
			SortIndex:             len(out),
			UpdateIntervalMinutes: updateInterval,
			TimeoutSeconds:        timeoutSeconds,
			MaxResponseBytes:      maxBytes,
			MaxRedirects:          maxRedirects,
		})
	}
	return out, nil
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// SetExternalLinksForRecord replaces a client's entire external-link set.
func (s *ClientService) SetExternalLinksForRecord(id int, inputs []ExternalLinkInput) error {
	rows, err := normalizeExternalLinks(inputs)
	if err != nil {
		return err
	}
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var existing []model.ClientExternalLink
		if err := tx.Where("client_id = ?", id).Find(&existing).Error; err != nil {
			return err
		}
		byID := make(map[int]model.ClientExternalLink, len(existing))
		for _, row := range existing {
			byID[row.Id] = row
		}
		keep := make([]int, 0, len(rows))
		for i := range rows {
			rows[i].ClientId = id
			if rows[i].Id > 0 {
				old, ok := byID[rows[i].Id]
				if !ok {
					return common.NewError("external source does not belong to client")
				}
				rows[i].LastGoodJSON = old.LastGoodJSON
				rows[i].LastSuccessAt = old.LastSuccessAt
				rows[i].LastAttemptAt = old.LastAttemptAt
				rows[i].LastError = old.LastError
				rows[i].LastHTTPStatus = old.LastHTTPStatus
				rows[i].RemoteETag = old.RemoteETag
				rows[i].RemoteLastModified = old.RemoteLastModified
			}
			if err := tx.Save(&rows[i]).Error; err != nil {
				return err
			}
			keep = append(keep, rows[i].Id)
		}
		query := tx.Where("client_id = ?", id)
		if len(keep) > 0 {
			query = query.Where("id NOT IN ?", keep)
		}
		if err := query.Delete(&model.ClientExternalLink{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *ClientService) SetExternalLinksByEmail(email string, inputs []ExternalLinkInput) error {
	if strings.TrimSpace(email) == "" {
		return common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return err
	}
	return s.SetExternalLinksForRecord(rec.Id, inputs)
}

func (s *ClientService) RefreshExternalJSONSource(id int) (*model.ClientExternalLink, error) {
	db := database.GetDB()
	var source model.ClientExternalLink
	if err := db.First(&source, id).Error; err != nil {
		return nil, err
	}
	if source.Kind != model.ExternalLinkKindJSONSubscription {
		return nil, common.NewError("external source is not a remote JSON subscription")
	}
	now := time.Now().UnixMilli()
	if err := db.Model(&model.ClientExternalLink{}).Where("id = ?", id).Updates(map[string]any{
		"last_attempt_at": now,
		"last_error":      "",
	}).Error; err != nil {
		return nil, err
	}
	result, fetchErr := securefetch.FetchJSON(context.Background(), source.Value, securefetch.Options{
		Timeout:      time.Duration(source.TimeoutSeconds) * time.Second,
		MaxBytes:     source.MaxResponseBytes,
		MaxRedirects: source.MaxRedirects,
		ETag:         source.RemoteETag,
		LastModified: source.RemoteLastModified,
	})
	if fetchErr != nil {
		status := 0
		if result != nil {
			status = result.StatusCode
		}
		publicErr := sanitizeExternalSourceError(fetchErr)
		_ = db.Model(&model.ClientExternalLink{}).Where("id = ?", id).Updates(map[string]any{
			"last_attempt_at":  now,
			"last_error":       publicErr.Error(),
			"last_http_status": status,
		}).Error
		return nil, publicErr
	}
	if result.NotModified && strings.TrimSpace(source.LastGoodJSON) == "" {
		result, fetchErr = securefetch.FetchJSON(context.Background(), source.Value, securefetch.Options{
			Timeout:      time.Duration(source.TimeoutSeconds) * time.Second,
			MaxBytes:     source.MaxResponseBytes,
			MaxRedirects: source.MaxRedirects,
		})
		if fetchErr != nil {
			publicErr := sanitizeExternalSourceError(fetchErr)
			_ = db.Model(&model.ClientExternalLink{}).Where("id = ?", id).Updates(map[string]any{
				"last_attempt_at": now,
				"last_error":      publicErr.Error(),
			}).Error
			return nil, publicErr
		}
	}
	updates := map[string]any{
		"last_attempt_at":  now,
		"last_success_at":  now,
		"last_error":       "",
		"last_http_status": result.StatusCode,
	}
	if result.ETag != "" {
		updates["remote_etag"] = result.ETag
	}
	if result.LastModified != "" {
		updates["remote_last_modified"] = result.LastModified
	}
	if !result.NotModified {
		if _, err := clientconfig.Parse(result.Body); err != nil {
			publicErr := sanitizeExternalSourceError(err)
			_ = db.Model(&model.ClientExternalLink{}).Where("id = ?", id).Updates(map[string]any{
				"last_attempt_at":  now,
				"last_error":       publicErr.Error(),
				"last_http_status": result.StatusCode,
			}).Error
			return nil, publicErr
		}
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, result.Body, "", "  "); err != nil {
			return nil, err
		}
		updates["last_good_json"] = formatted.String()
	}
	if err := db.Model(&model.ClientExternalLink{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := db.First(&source, id).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func (s *ClientService) RefreshDueExternalJSONSources(ctx context.Context) error {
	var sources []model.ClientExternalLink
	if err := database.GetDB().
		Where("kind = ? AND enabled = ?", model.ExternalLinkKindJSONSubscription, true).
		Find(&sources).Error; err != nil {
		return err
	}
	now := time.Now()
	var refreshErrors []error
	for _, source := range sources {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		interval := time.Duration(source.UpdateIntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = time.Hour
		}
		if source.LastAttemptAt > 0 && now.Sub(time.UnixMilli(source.LastAttemptAt)) < interval {
			continue
		}
		if _, err := s.RefreshExternalJSONSource(source.Id); err != nil {
			refreshErrors = append(refreshErrors, err)
		}
	}
	return errors.Join(refreshErrors...)
}

func sanitizeExternalSourceError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "https"):
		return common.NewError("remote JSON subscription must use HTTPS")
	case strings.Contains(message, "disallowed address"), strings.Contains(message, "host is not allowed"):
		return common.NewError("remote JSON subscription target is not allowed")
	case strings.Contains(message, "size"), strings.Contains(message, "too large"):
		return common.NewError("remote JSON subscription exceeds the configured size limit")
	case strings.Contains(message, "content type"):
		return common.NewError("remote JSON subscription returned a non-JSON content type")
	case strings.Contains(message, "status"):
		return common.NewError("remote JSON subscription returned an unsuccessful HTTP status")
	case strings.Contains(message, "redirect"):
		return common.NewError("remote JSON subscription exceeded the redirect limit")
	case strings.Contains(message, "line"), strings.Contains(message, "json"):
		return common.NewError(err.Error())
	default:
		return common.NewError("remote JSON subscription refresh failed")
	}
}
