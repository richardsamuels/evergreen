package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	notificationsStatsCollectorJobName = "notifications-stats-collector"
)

func init() {
	registry.AddJobType(notificationsStatsCollectorJobName,
		func() amboy.Job { return makeNotificationsStatsCollector() })
}

type notificationsStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

func makeNotificationsStatsCollector() *notificationsStatsCollector {
	j := &notificationsStatsCollector{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    notificationsStatsCollectorJobName,
				Version: 0,
			},
		},
		logger: logging.MakeGrip(grip.GetSender()),
	}
	j.SetDependency(dependency.NewAlways())
	j.SetID(fmt.Sprintf("%s:%s:%d", notificationsStatsCollectorJobName, time.Now().String(), job.GetNumber()))

	return j
}

func NewNotificationStatsCollector(id string) amboy.Job {
	job := makeNotificationsStatsCollector()
	job.SetID(id)

	return job
}

func (j *notificationsStatsCollector) Run(_ context.Context) {
	defer j.MarkComplete()

	e, err := event.FindLastProcessedEvent()
	j.AddError(errors.Wrap(err, "failed to fetch most recently processed event"))
	if j.HasErrors() {
		return
	}

	nUnprocessed, err := event.CountUnprocessedEvents()
	j.AddError(errors.Wrap(err, "failed to count unprocessed events"))
	if j.HasErrors() {
		return
	}

	stats, err := notification.CollectUnsentNotificationStats()
	j.AddError(errors.Wrap(err, "failed to collect notification stats"))
	if j.HasErrors() {
		return
	}

	pendingByType := message.Fields{}

	for k, v := range stats {
		pendingByType[k] = v
	}

	data := message.Fields{
		"unprocessed_events":            nUnprocessed,
		"pending_notifications_by_type": pendingByType,
	}
	if e != nil {
		data["last_processed_at"] = e.ProcessedAt
	}

	j.logger.Info(data)
}