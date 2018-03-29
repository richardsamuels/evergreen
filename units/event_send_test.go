package units

import (
	"context"
	"crypto/hmac"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventNotificationSuite struct {
	suite.Suite

	webhook notification.Notification
	email   notification.Notification
}

func TestEventNotificationJob(t *testing.T) {
	suite.Run(t, &eventNotificationSuite{})
}

func (s *eventNotificationSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *eventNotificationSuite) SetupTest() {
	s.NoError(db.ClearCollections(notification.NotificationsCollection, evergreen.ConfigCollection))
	s.webhook = notification.Notification{
		ID: bson.NewObjectId(),
		Subscriber: event.Subscriber{
			Type: event.EvergreenWebhookSubscriberType,
			Target: event.WebhookSubscriber{
				URL:    "http://127.0.0.1:12345",
				Secret: []byte("memes"),
			},
		},
		Payload: "o hai",
	}
	s.email = notification.Notification{
		ID: bson.NewObjectId(),
		Subscriber: event.Subscriber{
			Type:   event.EmailSubscriberType,
			Target: "o@hai.hai",
		},
		Payload: notification.EmailPayload{
			Subject: "o hai",
			Body:    "i'm a notification",
		},
	}

	s.NoError(notification.InsertMany(s.webhook, s.email))
}

type mockWebhookHandler struct {
	secret []byte

	body []byte
	err  error
}

func (m *mockWebhookHandler) error(outErr error, w http.ResponseWriter) {
	if outErr == nil {
		return
	}
	w.WriteHeader(http.StatusBadRequest)

	_, err := w.Write([]byte(outErr.Error()))
	grip.Error(err)
	m.err = outErr
}

func (m *mockWebhookHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if req.Method != http.MethodPost {
		m.error(errors.Errorf("expected method POST, got %s", req.Method), w)
		return
	}

	mid := req.Header.Get(evergreenNotificationIDHeader)
	if len(mid) == 0 {
		m.error(errors.New("no message id"), w)
		return
	}
	sig := []byte(req.Header.Get(evergreenHMACHeader))
	if len(sig) == 0 {
		m.error(errors.New("no signature"), w)
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		m.error(err, w)
		return
	}
	hash, err := calculateHMACHash(m.secret, body)
	if err != nil {
		m.error(err, w)
		return
	}

	if !hmac.Equal([]byte(hash), sig) {
		m.error(errors.Errorf("expected signature: %s, got %s", sig, hash), w)
		return
	}
	m.body = body

	w.WriteHeader(http.StatusNoContent)
	grip.Info(message.Fields{
		"message":   fmt.Sprintf("received %s", mid),
		"signature": string(sig),
		"body":      string(body),
	})
}

func (s *eventNotificationSuite) notificationHasError(id bson.ObjectId, pattern string) time.Time {
	n, err := notification.Find(id)
	s.Require().NoError(err)
	s.Require().NotNil(n)

	if len(pattern) == 0 {
		s.Empty(n.Error)

	} else {
		match, err := regexp.MatchString(pattern, n.Error)
		s.NoError(err)
		s.True(match)
	}

	return n.SentAt
}

func (s *eventNotificationSuite) TestDegradedMode() {
	flags := evergreen.ServiceFlags{
		JIRANotificationsDisabled:    true,
		SlackNotificationsDisabled:   true,
		EmailNotificationsDisabled:   true,
		WebhookNotificationsDisabled: true,
		GithubStatusAPIDisabled:      true,
		BackgroundStatsDisabled:      true,
	}
	s.NoError(flags.Set())

	job := newEventNotificationJob(s.webhook.ID)
	job.Run()
	s.EqualError(job.Error(), "sender is disabled, not sending notification")

	s.NotZero(s.notificationHasError(s.webhook.ID, "sender is disabled, not sending notification"))

	job = newEventNotificationJob(s.email.ID)
	job.Run()
	s.EqualError(job.Error(), "sender is disabled, not sending notification")

	s.NotZero(s.notificationHasError(s.webhook.ID, "sender is disabled, not sending notification"))
}

func (s *eventNotificationSuite) TestEvergreenWebhookWithDeadServer() {
	job := newEventNotificationJob(s.webhook.ID)
	job.Run()
	s.Require().NotNil(job.Error())
	errMsg := job.Error().Error()

	pattern := "evergreen-webhook failed to send webhook data: Post http://127.0.0.1:12345: dial tcp 127.0.0.1:12345: [a-zA-Z]+: connection refused"
	s.True(regexp.MatchString(pattern, errMsg))
	s.NotZero(s.notificationHasError(s.webhook.ID, pattern))
}

func (s *eventNotificationSuite) TestEvergreenWebhook() {
	job := newEventNotificationJob(s.webhook.ID)

	handler := &mockWebhookHandler{
		secret: []byte("memes"),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.NoError(err)
	s.NoError(db.UpdateId(notification.NotificationsCollection, s.webhook.ID, bson.M{
		"$set": bson.M{
			"subscriber.target.url": "http://" + ln.Addr().String(),
		},
	}))

	go func() {
		err := http.Serve(ln, handler)
		grip.Info(err)
	}()

	job.Run()
	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.webhook.ID, ""))
	s.Nil(job.Error())

	s.NoError(ln.Close())
}

func (s *eventNotificationSuite) TestEmail() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()

	addr := strings.Split(ln.Addr().String(), ":")
	s.Require().Len(addr, 2)

	port, err := strconv.Atoi(addr[1])
	s.Require().NoError(err)

	job := newEventNotificationJob(s.email.ID).(*eventNotificationJob)
	job.settings, err = evergreen.GetConfig()
	s.NoError(err)
	job.settings.Notify = evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			From:     "evergreen@example.com",
			Server:   "127.0.0.1",
			Port:     port,
			Username: "much",
			Password: "security",
		},
	}
	body := make(chan string, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go smtpServer(ctx, ln, body)

	job.Run()

	bodyText := <-body

	s.NotEmpty(bodyText)

	email := parseEmailBody(bodyText)
	s.Equal("<o@hai.hai>", email["To"])
	s.Equal("o hai", email["Subject"])
	s.Equal("i'm a notification", string(email["body"]))

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.email.ID, ""))
	s.Nil(job.Error())
}

func (s *eventNotificationSuite) TestEvergreenWebhookWithBadSecret() {
	job := newEventNotificationJob(s.webhook.ID)

	handler := &mockWebhookHandler{
		secret: []byte("somethingelse"),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()
	s.NoError(db.UpdateId(notification.NotificationsCollection, s.webhook.ID, bson.M{
		"$set": bson.M{
			"subscriber.target.url": "http://" + ln.Addr().String(),
		},
	}))

	go func() {
		err := http.Serve(ln, handler)
		if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
			s.T().Errorf("unexpected error: %+v", err)
		}
	}()

	job.Run()

	s.EqualError(job.Error(), "evergreen-webhook response status was 400")
	s.NotZero(s.notificationHasError(s.webhook.ID, "evergreen-webhook response status was 400"))
	s.Error(job.Error())
}

func (s *eventNotificationSuite) TestEmailWithUnreachableSMTP() {
	var err error
	job := newEventNotificationJob(s.email.ID).(*eventNotificationJob)
	job.settings, err = evergreen.GetConfig()
	s.NoError(err)
	job.settings.Notify = evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			From:     "evergreen@example.com",
			Server:   "127.0.0.1",
			Port:     12345,
			Username: "much",
			Password: "security",
		},
	}

	job.Run()
	s.Require().Error(job.Error())

	errMsg := job.Error().Error()
	pattern := "email settings are invalid: dial tcp 127.0.0.1:12345: [a-zA-Z]+: connection refused"
	s.True(regexp.MatchString(pattern, errMsg))
	s.NotZero(s.notificationHasError(s.email.ID, pattern))
}

func (s *eventNotificationSuite) TestSlack() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()

	addr := strings.Split(ln.Addr().String(), ":")
	s.Require().Len(addr, 2)

	port, err := strconv.Atoi(addr[1])
	s.Require().NoError(err)

	job := newEventNotificationJob(s.email.ID).(*eventNotificationJob)
	job.settings, err = evergreen.GetConfig()
	s.NoError(err)
	job.settings.Notify = evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			From:     "evergreen@example.com",
			Server:   "127.0.0.1",
			Port:     port,
			Username: "much",
			Password: "security",
		},
	}
	body := make(chan string, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go smtpServer(ctx, ln, body)

	job.Run()

	bodyText := <-body

	s.NotEmpty(bodyText)

	email := parseEmailBody(bodyText)
	s.Equal("<o@hai.hai>", email["To"])
	s.Equal("o hai", email["Subject"])
	s.Equal("i'm a notification", string(email["body"]))

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.email.ID, ""))
	s.Nil(job.Error())
}
