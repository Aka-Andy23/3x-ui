package job

import (
	"context"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type ExternalJSONSubscriptionJob struct {
	clientService service.ClientService
}

func NewExternalJSONSubscriptionJob() *ExternalJSONSubscriptionJob {
	return &ExternalJSONSubscriptionJob{}
}

func (j *ExternalJSONSubscriptionJob) Run() {
	if err := j.clientService.RefreshDueExternalJSONSources(context.Background()); err != nil {
		logger.Warning("external JSON subscription refresh completed with errors")
	}
}
